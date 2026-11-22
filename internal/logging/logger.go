package logging

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	DefaultMaxBytes   = 5 * 1024 * 1024 // 5 MB per log file
	DefaultMaxBackups = 3               // Keep vex.jsonl.1 .. .3
)

// LogEntry represents a parsed structured JSONL log line for local UI inspection.
type LogEntry struct {
	Time      string         `json:"time"`
	Level     string         `json:"level"`
	Msg       string         `json:"msg"`
	Service   string         `json:"service,omitempty"`
	Component string         `json:"component,omitempty"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// LogDiagnostics summarizes local logging state for the Settings UI.
type LogDiagnostics struct {
	FilePath    string `json:"filePath"`
	Level       string `json:"level"`
	SizeBytes   int64  `json:"sizeBytes"`
	MetricsAddr string `json:"metricsAddr,omitempty"`
}

// Metrics tracks lightweight Prometheus-compatible counters and gauges with zero dependencies.
type Metrics struct {
	SearchesTotal     atomic.Int64
	ActionsTotal      atomic.Int64
	ActionErrorsTotal atomic.Int64
	MCPRequestsTotal  atomic.Int64
	ModuleSyncsTotal  atomic.Int64
	ActiveCards       atomic.Int64
	EnabledModules    atomic.Int64
}

// WritePrometheus writes all counters and gauges in Prometheus text exposition format (0.0.4).
func (m *Metrics) WritePrometheus(w io.Writer) {
	if m == nil {
		return
	}
	fmt.Fprintf(w, "# HELP vex_searches_total Total search queries executed.\n")
	fmt.Fprintf(w, "# TYPE vex_searches_total counter\n")
	fmt.Fprintf(w, "vex_searches_total %d\n", m.SearchesTotal.Load())

	fmt.Fprintf(w, "# HELP vex_actions_total Total card actions executed.\n")
	fmt.Fprintf(w, "# TYPE vex_actions_total counter\n")
	fmt.Fprintf(w, "vex_actions_total %d\n", m.ActionsTotal.Load())

	fmt.Fprintf(w, "# HELP vex_action_errors_total Total card action execution failures.\n")
	fmt.Fprintf(w, "# TYPE vex_action_errors_total counter\n")
	fmt.Fprintf(w, "vex_action_errors_total %d\n", m.ActionErrorsTotal.Load())

	fmt.Fprintf(w, "# HELP vex_mcp_requests_total Total MCP HTTP requests handled.\n")
	fmt.Fprintf(w, "# TYPE vex_mcp_requests_total counter\n")
	fmt.Fprintf(w, "vex_mcp_requests_total %d\n", m.MCPRequestsTotal.Load())

	fmt.Fprintf(w, "# HELP vex_module_syncs_total Total composable module compilations.\n")
	fmt.Fprintf(w, "# TYPE vex_module_syncs_total counter\n")
	fmt.Fprintf(w, "vex_module_syncs_total %d\n", m.ModuleSyncsTotal.Load())

	fmt.Fprintf(w, "# HELP vex_active_cards Current number of indexed SearchCards.\n")
	fmt.Fprintf(w, "# TYPE vex_active_cards gauge\n")
	fmt.Fprintf(w, "vex_active_cards %d\n", m.ActiveCards.Load())

	fmt.Fprintf(w, "# HELP vex_enabled_modules Current number of enabled knowledge modules.\n")
	fmt.Fprintf(w, "# TYPE vex_enabled_modules gauge\n")
	fmt.Fprintf(w, "vex_enabled_modules %d\n", m.EnabledModules.Load())
}

// rotatingWriter is a concurrency-safe io.WriteCloser that rotates a local log file by size.
type rotatingWriter struct {
	mu         sync.Mutex
	path       string
	maxBytes   int64
	maxBackups int
	file       *os.File
	size       int64
}

func newRotatingWriter(path string, maxBytes int64, maxBackups int) (*rotatingWriter, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	rw := &rotatingWriter{
		path:       path,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
	}
	if err := rw.openLocked(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (w *rotatingWriter) openLocked() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.openLocked(); err != nil {
			return 0, err
		}
	}
	if w.size+int64(len(p)) > w.maxBytes && w.size > 0 {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	// Shift .2 -> .3, .1 -> .2, base -> .1
	for i := w.maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		_ = os.Rename(src, dst)
	}
	_ = os.Rename(w.path, w.path+".1")
	return w.openLocked()
}

func (w *rotatingWriter) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

func (w *rotatingWriter) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if err := os.WriteFile(w.path, nil, 0o600); err != nil {
		return err
	}
	return w.openLocked()
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// Manager owns the structured slog.Logger, rotating JSONL file writer, dynamic level, and Prometheus Metrics.
type Manager struct {
	filePath string
	writer   *rotatingWriter
	levelVar *slog.LevelVar
	logger   *slog.Logger
	Metrics  *Metrics
}

// NewManager initializes a Loki/Grafana-ready JSONL logger at <logDir>/vex.jsonl.
// Never writes to stdout so --mcp-stdio JSON-RPC remains completely clean.
func NewManager(logDir, level string, maxBytes int64, maxBackups int) (*Manager, error) {
	filePath := filepath.Join(logDir, "vex.jsonl")
	rw, err := newRotatingWriter(filePath, maxBytes, maxBackups)
	if err != nil {
		return nil, fmt.Errorf("init log writer: %w", err)
	}

	lvlVar := &slog.LevelVar{}
	lvlVar.Set(ParseLevel(level))

	handler := slog.NewJSONHandler(rw, &slog.HandlerOptions{
		Level: lvlVar,
	})
	baseLogger := slog.New(handler).With("service", "vex")

	return &Manager{
		filePath: filePath,
		writer:   rw,
		levelVar: lvlVar,
		logger:   baseLogger,
		Metrics:  &Metrics{},
	}, nil
}

// ParseLevel converts a string level ("debug", "info", "warn", "error") to slog.Level.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Logger returns the root structured slog.Logger.
func (m *Manager) Logger() *slog.Logger {
	return m.logger
}

// Component returns a child logger tagged with a low-cardinality Loki component label.
func (m *Manager) Component(name string) *slog.Logger {
	return m.logger.With("component", name)
}

// SetLevel dynamically updates the active log level without restarting the app.
func (m *Manager) SetLevel(level string) string {
	parsed := ParseLevel(level)
	m.levelVar.Set(parsed)
	return strings.ToLower(parsed.String())
}

// Level returns the current level name ("debug", "info", "warn", "error").
func (m *Manager) Level() string {
	return strings.ToLower(m.levelVar.Level().String())
}

// FilePath returns the absolute path to the active JSONL log file.
func (m *Manager) FilePath() string {
	return m.filePath
}

// Diagnostics returns summary metadata for the Settings UI.
func (m *Manager) Diagnostics(metricsAddr string) LogDiagnostics {
	return LogDiagnostics{
		FilePath:    m.filePath,
		Level:       m.Level(),
		SizeBytes:   m.writer.Size(),
		MetricsAddr: metricsAddr,
	}
}

// Clear truncates the active log file.
func (m *Manager) Clear() error {
	return m.writer.Truncate()
}

// Close flushes and closes the underlying log file.
func (m *Manager) Close() error {
	if m.writer != nil {
		return m.writer.Close()
	}
	return nil
}

// ReadRecentEntries reads up to limit of the most recent structured JSONL log lines (newest first).
func (m *Manager) ReadRecentEntries(limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	f, err := os.Open(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	entries := make([]LogEntry, 0, len(lines))
	// Return newest first for convenient UI inspection
	for i := len(lines) - 1; i >= 0; i-- {
		var raw map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &raw); err != nil {
			continue
		}
		entry := LogEntry{
			Attrs: make(map[string]any),
		}
		for k, v := range raw {
			switch k {
			case "time":
				entry.Time = fmt.Sprintf("%v", v)
			case "level":
				entry.Level = fmt.Sprintf("%v", v)
			case "msg":
				entry.Msg = fmt.Sprintf("%v", v)
			case "service":
				entry.Service = fmt.Sprintf("%v", v)
			case "component":
				entry.Component = fmt.Sprintf("%v", v)
			default:
				entry.Attrs[k] = v
			}
		}
		if len(entry.Attrs) == 0 {
			entry.Attrs = nil
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
