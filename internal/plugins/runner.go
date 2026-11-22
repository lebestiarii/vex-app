package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"vex/internal/models"
	"vex/internal/search"
)

var actionPlaceholderRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// ActionResult captures the outcome of executing a CardAction.
type ActionResult struct {
	Success        bool           `json:"success"`
	ActionType     string         `json:"actionType"`
	ResolvedTarget string         `json:"resolvedTarget"`
	Output         string         `json:"output,omitempty"`
	Error          string         `json:"error,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	DurationMs     int64          `json:"durationMs"`
	OpenAIMode     bool           `json:"openAiMode,omitempty"`
}

// Runner executes SearchCard actions across macOS, Windows, and Linux.
type Runner struct {
	HTTPClient    *http.Client
	ShellTimeout  time.Duration
	OpenFunc      func(ctx context.Context, kind, target string) error
	ClipboardFunc func(ctx context.Context, text string) error
}

// NewRunner returns a cross-platform Action Runner with sensible defaults.
func NewRunner() *Runner {
	return &Runner{
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
		ShellTimeout: 15 * time.Second,
	}
}

// InterpolateTemplate resolves {{query}}, {{raw}}, {{arg1}}, {{param}}, {{data.key}}, and {{card.title}} placeholders.
func InterpolateTemplate(tmpl string, card models.SearchCard, parsed search.ParsedQuery, extraArgs map[string]string) string {
	if !strings.Contains(tmpl, "{{") {
		return tmpl
	}

	return actionPlaceholderRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := actionPlaceholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		key := strings.TrimSpace(sub[1])
		lower := strings.ToLower(key)

		// 1. Explicit UI form / extra args override everything
		if extraArgs != nil {
			if v, ok := extraArgs[key]; ok {
				return v
			}
			if v, ok := extraArgs[lower]; ok {
				return v
			}
		}

		// 2. Inline key:value params from query parser
		if parsed.Params != nil {
			if v, ok := parsed.Params[lower]; ok {
				return v
			}
		}

		// 3. Built-in query variables
		switch lower {
		case "query", "text":
			return parsed.Text
		case "raw":
			return parsed.Raw
		case "modifier":
			return parsed.Modifier
		case "collection":
			return card.Collection
		case "card.title", "title":
			return card.Title
		case "card.subtitle", "subtitle":
			return card.Subtitle
		}

		// 4. Positional arguments: {{arg1}}, {{arg2}}, ...
		if strings.HasPrefix(lower, "arg") {
			if n, err := strconv.Atoi(strings.TrimPrefix(lower, "arg")); err == nil && n >= 1 && n <= len(parsed.Args) {
				return parsed.Args[n-1]
			}
		}

		// 5. Card structured data: {{data.host}} or direct {{host}}
		dataKey := strings.TrimPrefix(key, "data.")
		if card.Data != nil {
			if v, ok := card.Data[dataKey]; ok && v != nil {
				return fmt.Sprintf("%v", v)
			}
			for k, v := range card.Data {
				if strings.EqualFold(k, dataKey) && v != nil {
					return fmt.Sprintf("%v", v)
				}
			}
		}

		return ""
	})
}

// Execute finds the requested action (or the first action if actionID is empty) and runs it.
func (r *Runner) Execute(ctx context.Context, card models.SearchCard, actionID string, parsed search.ParsedQuery, extraArgs map[string]string) ActionResult {
	start := time.Now()
	allActions := make([]models.CardAction, 0, len(card.Actions))
	allActions = append(allActions, card.Actions...)
	for _, sec := range card.Sections {
		allActions = append(allActions, sec.Actions...)
	}

	if len(allActions) == 0 {
		return ActionResult{
			Success:    false,
			Error:      "card has no executable actions",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	var action *models.CardAction
	if actionID == "" {
		action = &allActions[0]
	} else {
		for i := range allActions {
			if allActions[i].ID == actionID {
				action = &allActions[i]
				break
			}
		}
	}
	if action == nil {
		return ActionResult{
			Success:    false,
			Error:      fmt.Sprintf("action %q not found on card %q", actionID, card.ID),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	resolved := strings.TrimSpace(InterpolateTemplate(action.Target, card, parsed, extraArgs))
	res := ActionResult{
		ActionType:     action.Type,
		ResolvedTarget: resolved,
	}

	switch action.Type {
	case "open_url", "open_pwa", "launch_app":
		if resolved == "" {
			res.Error = "resolved target is empty"
			break
		}
		if action.Type == "open_url" || action.Type == "open_pwa" {
			if err := validateHTTPURL(resolved); err != nil {
				res.Error = err.Error()
				break
			}
		}
		var err error
		if r.OpenFunc != nil {
			err = r.OpenFunc(ctx, action.Type, resolved)
		} else {
			err = defaultOSOpen(ctx, action.Type, resolved)
		}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
			res.Output = fmt.Sprintf("Opened %s", resolved)
		}

	case "copy":
		var err error
		if r.ClipboardFunc != nil {
			err = r.ClipboardFunc(ctx, resolved)
		} else {
			err = defaultCopyClipboard(ctx, resolved)
		}
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
			res.Output = resolved
		}

	case "http":
		r.execHTTP(ctx, resolved, &res)

	case "ai_prompt":
		res.Success = true
		res.Output = resolved
		res.OpenAIMode = true

	default:
		res.Error = fmt.Sprintf("unsupported action type %q", action.Type)
	}

	res.DurationMs = time.Since(start).Milliseconds()
	return res
}

func (r *Runner) execHTTP(ctx context.Context, resolved string, res *ActionResult) {
	if resolved == "" {
		res.Error = "http target URL is empty"
		return
	}
	method := http.MethodGet
	targetURL := resolved
	if parts := strings.SplitN(resolved, " ", 2); len(parts) == 2 {
		m := strings.ToUpper(parts[0])
		if m == "GET" || m == "POST" || m == "PUT" || m == "DELETE" || m == "PATCH" {
			method = m
			targetURL = strings.TrimSpace(parts[1])
		}
	}

	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, nil)
	if err != nil {
		res.Error = err.Error()
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		res.Error = err.Error()
		return
	}
	trimmed := bytes.TrimSpace(body)
	res.Output = string(trimmed)
	res.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !res.Success {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var obj map[string]any
		if json.Unmarshal(trimmed, &obj) == nil {
			res.Data = obj
		}
	}
}

// validateHTTPURL enforces strict http:// or https:// URLs and rejects shell metacharacters
// that could cause command injection in Windows `cmd /c start` or macOS `open`.
func validateHTTPURL(raw string) error {
	if strings.ContainsAny(raw, "\r\n\x00|&;^`$") {
		return fmt.Errorf("unsafe characters in URL %q", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("blocked URL scheme %q (only http and https are allowed)", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("URL %q is missing a host", raw)
	}
	return nil
}

func defaultOSOpen(ctx context.Context, kind, target string) error {
	switch runtime.GOOS {
	case "darwin":
		switch kind {
		case "launch_app":
			return exec.CommandContext(ctx, "open", "-a", target).Start()
		case "open_pwa":
			// Try opening in standalone Chrome app window, fall back to default browser.
			if err := exec.CommandContext(ctx, "open", "-na", "Google Chrome", "--args", "--app="+target).Start(); err == nil {
				return nil
			}
			return exec.CommandContext(ctx, "open", target).Start()
		default:
			return exec.CommandContext(ctx, "open", target).Start()
		}
	case "windows":
		switch kind {
		case "open_pwa":
			if err := exec.CommandContext(ctx, "cmd", "/c", "start", "msedge", "--app="+target).Start(); err == nil {
				return nil
			}
			return exec.CommandContext(ctx, "cmd", "/c", "start", "", target).Start()
		default:
			return exec.CommandContext(ctx, "cmd", "/c", "start", "", target).Start()
		}
	default:
		return exec.CommandContext(ctx, "xdg-open", target).Start()
	}
}

func defaultCopyClipboard(ctx context.Context, text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbcopy")
	case "windows":
		cmd = exec.CommandContext(ctx, "clip")
	default:
		cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
