package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// UpdateEvent describes an incremental index change triggered by a JSON file update.
type UpdateEvent struct {
	Collection string `json:"collection"`
	FilePath   string `json:"filePath"`
	CardCount  int    `json:"cardCount"`
	Deleted    bool   `json:"deleted"`
}

// Watcher monitors a directory of JSON collection files (and an optional patterns directory)
// and keeps the SQLite Store and PatternEngine synchronized in real time.
type Watcher struct {
	store             *Store
	dataDir           string
	patternsDir       string
	debounce          time.Duration
	onUpdate          func(UpdateEvent)
	onPatternsChanged func()
	logger            *slog.Logger

	mu     sync.Mutex
	fsw    *fsnotify.Watcher
	cancel context.CancelFunc
	done   chan struct{}
}

// NewWatcher creates a JSON directory watcher bound to store.
func NewWatcher(store *Store, dataDir string, onUpdate func(UpdateEvent)) *Watcher {
	return &Watcher{
		store:    store,
		dataDir:  dataDir,
		debounce: 80 * time.Millisecond,
		onUpdate: onUpdate,
		logger:   slog.Default(),
	}
}

// SetPatternsWatcher registers an optional ~/.vex/patterns directory for hot-reloading PatternRules.
func (w *Watcher) SetPatternsWatcher(patternsDir string, onChange func()) {
	w.mu.Lock()
	w.patternsDir = strings.TrimSpace(patternsDir)
	w.onPatternsChanged = onChange
	w.mu.Unlock()
}

// SyncAll scans dataDir and indexes every .json collection file synchronously.
func (w *Watcher) SyncAll(ctx context.Context) (int, error) {
	if err := os.MkdirAll(w.dataDir, 0o755); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(w.dataDir)
	if err != nil {
		return 0, err
	}

	totalCards := 0
	for _, entry := range entries {
		if entry.IsDir() || !isCollectionJSON(entry.Name()) {
			continue
		}
		fullPath := filepath.Join(w.dataDir, entry.Name())
		count, err := w.indexFile(ctx, fullPath)
		if err != nil {
			w.logger.Warn("failed to index collection file", "path", fullPath, "error", err)
			continue
		}
		totalCards += count
	}
	return totalCards, nil
}

// Start performs an initial SyncAll and begins watching dataDir for real-time changes.
func (w *Watcher) Start(parentCtx context.Context) error {
	if _, err := w.SyncAll(parentCtx); err != nil {
		return err
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	if err := fsw.Add(w.dataDir); err != nil {
		_ = fsw.Close()
		return fmt.Errorf("watch dir %s: %w", w.dataDir, err)
	}
	if w.patternsDir != "" {
		if err := os.MkdirAll(w.patternsDir, 0o755); err == nil {
			_ = fsw.Add(w.patternsDir)
		}
	}

	ctx, cancel := context.WithCancel(parentCtx)
	done := make(chan struct{})

	w.mu.Lock()
	w.fsw = fsw
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()

	go w.loop(ctx, fsw, done)
	return nil
}

// Stop shuts down the watcher and waits for its background loop to exit.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	cancel := w.cancel
	fsw := w.fsw
	done := w.done
	w.cancel = nil
	w.fsw = nil
	w.done = nil
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	var err error
	if fsw != nil {
		err = fsw.Close()
	}
	if done != nil {
		<-done
	}
	return err
}

func (w *Watcher) loop(ctx context.Context, fsw *fsnotify.Watcher, done chan struct{}) {
	defer close(done)

	var timerMu sync.Mutex
	timers := make(map[string]*time.Timer)

	stopAllTimers := func() {
		timerMu.Lock()
		for _, t := range timers {
			t.Stop()
		}
		timers = make(map[string]*time.Timer)
		timerMu.Unlock()
	}
	defer stopAllTimers()

	schedule := func(path string, removed bool) {
		timerMu.Lock()
		defer timerMu.Unlock()
		if existing, ok := timers[path]; ok {
			existing.Stop()
		}
		timers[path] = time.AfterFunc(w.debounce, func() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if removed {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					if err := w.store.DeleteBySourceFile(ctx, path); err != nil {
						w.logger.Warn("failed to delete cards for removed file", "path", path, "error", err)
						return
					}
					if w.onUpdate != nil {
						w.onUpdate(UpdateEvent{
							Collection: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
							FilePath:   path,
							CardCount:  0,
							Deleted:    true,
						})
					}
					return
				}
			}
			count, err := w.indexFile(ctx, path)
			if err != nil {
				w.logger.Warn("failed to re-index file", "path", path, "error", err)
				return
			}
			if w.onUpdate != nil {
				w.onUpdate(UpdateEvent{
					Collection: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
					FilePath:   path,
					CardCount:  count,
					Deleted:    false,
				})
			}
		})
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			if !isCollectionJSON(filepath.Base(event.Name)) {
				continue
			}
			if w.patternsDir != "" && filepath.Clean(filepath.Dir(event.Name)) == filepath.Clean(w.patternsDir) {
				if w.onPatternsChanged != nil {
					timerMu.Lock()
					if existing, ok := timers["__patterns__"]; ok {
						existing.Stop()
					}
					timers["__patterns__"] = time.AfterFunc(w.debounce, func() {
						select {
						case <-ctx.Done():
							return
						default:
							w.onPatternsChanged()
						}
					})
					timerMu.Unlock()
				}
				continue
			}
			switch {
			case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
				schedule(event.Name, true)
			case event.Has(fsnotify.Write) || event.Has(fsnotify.Create):
				schedule(event.Name, false)
			}
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			w.logger.Warn("fsnotify error", "error", err)
		}
	}
}

func (w *Watcher) indexFile(ctx context.Context, path string) (int, error) {
	col, cards, err := LoadCollectionFile(path)
	if err != nil {
		return 0, err
	}
	if err := w.store.ReplaceCollection(ctx, col, cards, path); err != nil {
		return 0, err
	}
	return len(cards), nil
}

func isCollectionJSON(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".json") && !strings.HasPrefix(lower, ".")
}
