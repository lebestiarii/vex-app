//go:build windows

package autostart

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	hkeyCurrentUser = uintptr(0x80000001)
	keyQueryValue   = 0x0001
	keySetValue     = 0x0002
	regSZ           = 1
	winErrFileNotFound = 2
	runSubKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
)

var (
	advapi32            = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW   = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey     = advapi32.NewProc("RegCloseKey")
)

type windowsManager struct {
	valueName string
	execPath  string
}

// NewDefaultManager returns a Windows HKCU Run-key Manager for appName.
func NewDefaultManager(appName string) Manager {
	name := strings.TrimSpace(appName)
	if name == "" {
		name = "Vex"
	}
	execPath, _ := currentExecutablePath()
	return &windowsManager{
		valueName: name,
		execPath:  execPath,
	}
}

func (m *windowsManager) IsEnabled() (bool, error) {
	subKeyPtr, err := syscall.UTF16PtrFromString(runSubKey)
	if err != nil {
		return false, err
	}
	valPtr, err := syscall.UTF16PtrFromString(m.valueName)
	if err != nil {
		return false, err
	}

	var hKey uintptr
	r1, _, _ := procRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(subKeyPtr)),
		0,
		uintptr(keyQueryValue),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return false, nil
	}
	defer procRegCloseKey.Call(hKey)

	var typ, size uint32
	r2, _, _ := procRegQueryValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(valPtr)),
		0,
		uintptr(unsafe.Pointer(&typ)),
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	if r2 == winErrFileNotFound {
		return false, nil
	}
	if r2 != 0 {
		return false, fmt.Errorf("RegQueryValueExW error %d", r2)
	}
	return size > 0, nil
}

func (m *windowsManager) SetEnabled(enabled bool) error {
	subKeyPtr, err := syscall.UTF16PtrFromString(runSubKey)
	if err != nil {
		return err
	}
	valPtr, err := syscall.UTF16PtrFromString(m.valueName)
	if err != nil {
		return err
	}

	var hKey uintptr
	r1, _, _ := procRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(subKeyPtr)),
		0,
		uintptr(keySetValue),
		uintptr(unsafe.Pointer(&hKey)),
	)
	if r1 != 0 {
		return fmt.Errorf("RegOpenKeyExW error %d", r1)
	}
	defer procRegCloseKey.Call(hKey)

	if !enabled {
		r2, _, _ := procRegDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(valPtr)))
		if r2 != 0 && r2 != winErrFileNotFound {
			return fmt.Errorf("RegDeleteValueW error %d", r2)
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
	quoted := `"` + m.execPath + `"`
	data, err := syscall.UTF16FromString(quoted)
	if err != nil {
		return err
	}
	r2, _, _ := procRegSetValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(valPtr)),
		0,
		uintptr(regSZ),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)*2),
	)
	if r2 != 0 {
		return fmt.Errorf("RegSetValueExW error %d", r2)
	}
	return nil
}
