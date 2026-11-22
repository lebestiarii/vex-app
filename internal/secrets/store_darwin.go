//go:build darwin

package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const securityBin = "/usr/bin/security"

type darwinStore struct{}

// NewDefaultStore returns the macOS Keychain-backed secret Store.
func NewDefaultStore() Store {
	return &darwinStore{}
}

func (s *darwinStore) Get(service, account string) (string, error) {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return "", errors.New("service and account are required")
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(securityBin, "find-generic-password", "-s", service, "-a", account, "-w")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain get %s/%s: %w (%s)", service, account, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

func (s *darwinStore) Set(service, account, secret string) error {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return errors.New("service and account are required")
	}
	if secret == "" {
		return s.Delete(service, account)
	}

	var stderr bytes.Buffer
	cmd := exec.Command(securityBin, "add-generic-password", "-U", "-s", service, "-a", account, "-w", secret)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain set %s/%s: %w (%s)", service, account, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *darwinStore) Delete(service, account string) error {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return errors.New("service and account are required")
	}

	var stderr bytes.Buffer
	cmd := exec.Command(securityBin, "delete-generic-password", "-s", service, "-a", account)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return nil
		}
		return fmt.Errorf("keychain delete %s/%s: %w (%s)", service, account, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
