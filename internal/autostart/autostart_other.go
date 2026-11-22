//go:build !darwin && !windows

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type xdgManager struct {
	appName  string
	execPath string
	dir      string
}

// NewDefaultManager returns an XDG autostart .desktop Manager.
func NewDefaultManager(appName string) Manager {
	home, _ := os.UserHomeDir()
	execPath, _ := currentExecutablePath()
	return &xdgManager{
		appName:  strings.TrimSpace(appName),
		execPath: execPath,
		dir:      filepath.Join(home, ".config", "autostart"),
	}
}

func (m *xdgManager) desktopPath() string {
	return filepath.Join(m.dir, sanitizeIdentifier(m.appName)+".desktop")
}

func (m *xdgManager) IsEnabled() (bool, error) {
	_, err := os.Stat(m.desktopPath())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (m *xdgManager) SetEnabled(enabled bool) error {
	target := m.desktopPath()
	if !enabled {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nX-GNOME-Autostart-enabled=true\n", m.appName, m.execPath)
	return os.WriteFile(target, []byte(entry), 0o644)
}
