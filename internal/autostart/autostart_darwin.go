//go:build darwin

package autostart

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type darwinManager struct {
	label     string
	execPath  string
	agentsDir string
}

// NewDefaultManager returns a macOS LaunchAgent Manager for appName.
func NewDefaultManager(appName string) Manager {
	home, _ := os.UserHomeDir()
	execPath, _ := currentExecutablePath()
	label := "com." + sanitizeIdentifier(appName) + ".launcher"
	return NewDarwinManager(label, execPath, filepath.Join(home, "Library", "LaunchAgents"))
}

// NewDarwinManager constructs a LaunchAgent Manager targeting an explicit directory (useful for tests).
func NewDarwinManager(label, execPath, agentsDir string) Manager {
	return &darwinManager{
		label:     strings.TrimSpace(label),
		execPath:  strings.TrimSpace(execPath),
		agentsDir: strings.TrimSpace(agentsDir),
	}
}

func (m *darwinManager) plistPath() string {
	return filepath.Join(m.agentsDir, m.label+".plist")
}

func (m *darwinManager) IsEnabled() (bool, error) {
	_, err := os.Stat(m.plistPath())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (m *darwinManager) SetEnabled(enabled bool) error {
	target := m.plistPath()
	if !enabled {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove launch agent plist: %w", err)
		}
		return nil
	}

	if m.execPath == "" {
		p, err := currentExecutablePath()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
		m.execPath = p
	}
	if err := os.MkdirAll(m.agentsDir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	content := renderLaunchAgentPlist(m.label, m.execPath)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write launch agent plist: %w", err)
	}
	return os.Rename(tmp, target)
}

func renderLaunchAgentPlist(label, execPath string) string {
	var labelBuf, execBuf strings.Builder
	_ = xml.EscapeText(&labelBuf, []byte(label))
	_ = xml.EscapeText(&execBuf, []byte(execPath))

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`, labelBuf.String(), execBuf.String())
}
