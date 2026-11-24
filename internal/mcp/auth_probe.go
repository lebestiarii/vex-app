package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// CLIAuthStatus reports read-only authentication readiness for local developer CLIs
// (gh, az) without ever extracting, reading, or caching bearer tokens.
type CLIAuthStatus struct {
	Tool          string `json:"tool"` // "gh" | "az"
	Installed     bool   `json:"installed"`
	Authenticated bool   `json:"authenticated"`
	Account       string `json:"account,omitempty"`
	HostOrTenant  string `json:"hostOrTenant,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// CommandRunner executes a read-only CLI diagnostic command and returns combined output.
type CommandRunner func(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)

func defaultCommandRunner(ctx context.Context, name string, args ...string) (string, string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// ProbeCLIAuth checks whether local `gh` (GitHub CLI) and `az` (Azure CLI) sessions
// are present and signed in. It strictly runs read-only identity checks (`gh auth status`,
// `az account show`) and NEVER invokes `gh auth token` or `az account get-access-token`.
func ProbeCLIAuth(ctx context.Context, runner CommandRunner) []CLIAuthStatus {
	if runner == nil {
		runner = defaultCommandRunner
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	return []CLIAuthStatus{
		probeGH(probeCtx, runner),
		probeAZ(probeCtx, runner),
	}
}

func probeGH(ctx context.Context, runner CommandRunner) CLIAuthStatus {
	st := CLIAuthStatus{Tool: "gh"}
	stdout, stderr, err := runner(ctx, "gh", "auth", "status")
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			st.Detail = "gh CLI not installed"
			return st
		}
		st.Installed = true
		st.Detail = "Not signed in (run `gh auth login`)"
		return st
	}

	st.Installed = true
	st.Authenticated = true
	for _, line := range strings.Split(combined, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "logged in to ") && strings.Contains(lower, " account ") {
			// Example: "✓ Logged in to github.com account octocat (keyring)"
			if idx := strings.Index(lower, "logged in to "); idx >= 0 {
				rest := trimmed[idx+len("logged in to "):]
				fields := strings.Fields(rest)
				if len(fields) >= 3 && strings.EqualFold(fields[1], "account") {
					st.HostOrTenant = fields[0]
					st.Account = fields[2]
				}
			}
			break
		}
	}
	if st.Account != "" {
		st.Detail = "Authenticated via local gh session"
	} else {
		st.Detail = "Authenticated"
	}
	return st
}

func probeAZ(ctx context.Context, runner CommandRunner) CLIAuthStatus {
	st := CLIAuthStatus{Tool: "az"}
	stdout, _, err := runner(ctx, "az", "account", "show", "--output", "json")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			st.Detail = "az CLI not installed"
			return st
		}
		st.Installed = true
		st.Detail = "Not signed in (run `az login`)"
		return st
	}

	st.Installed = true
	st.Authenticated = true

	var payload struct {
		Name     string `json:"name"`
		TenantID string `json:"tenantId"`
		User     struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &payload); jsonErr == nil {
		st.Account = strings.TrimSpace(payload.User.Name)
		st.HostOrTenant = strings.TrimSpace(payload.TenantID)
		if payload.Name != "" {
			st.Detail = "Subscription: " + strings.TrimSpace(payload.Name)
		} else {
			st.Detail = "Authenticated via local az session"
		}
	} else {
		st.Detail = "Authenticated"
	}
	return st
}

// RecommendedMCPPresets returns opt-in, localOnly-enforced ExternalServerConfig templates
// for official GitHub and Azure DevOps MCP servers. Credentials stay isolated inside the
// external MCP server process or OS CLI broker — never stored inside Vex.
func RecommendedMCPPresets() []ExternalServerConfig {
	return []ExternalServerConfig{
		{
			ID:        "github",
			Name:      "GitHub MCP Server",
			Transport: "stdio",
			Command:   "github-mcp-server",
			Args:      []string{"stdio"},
			Enabled:   false,
			LocalOnly: true,
		},
		{
			ID:        "azure-devops",
			Name:      "Azure DevOps MCP Server",
			Transport: "stdio",
			Command:   "npx",
			Args:      []string{"-y", "@azure-devops/mcp", "<your-org-name>"},
			Enabled:   false,
			LocalOnly: true,
		},
	}
}
