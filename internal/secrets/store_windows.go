//go:build windows

package secrets

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	winErrNotFound          = syscall.Errno(1168)
)

var (
	advapi32        = syscall.NewLazyDLL("advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsStore struct{}

// NewDefaultStore returns the Windows Credential Manager-backed secret Store.
func NewDefaultStore() Store {
	return &windowsStore{}
}

func targetName(service, account string) string {
	return strings.TrimSpace(service) + "/" + strings.TrimSpace(account)
}

func (s *windowsStore) Get(service, account string) (string, error) {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return "", errors.New("service and account are required")
	}

	targetPtr, err := syscall.UTF16PtrFromString(targetName(service, account))
	if err != nil {
		return "", err
	}

	var cred *credentialW
	r1, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&cred)),
	)
	if r1 == 0 {
		if errors.Is(callErr, winErrNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("wincred get %s/%s: %w", service, account, callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))

	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == nil {
		return "", nil
	}
	blob := unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)
	return string(blob), nil
}

func (s *windowsStore) Set(service, account, secret string) error {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return errors.New("service and account are required")
	}
	if secret == "" {
		return s.Delete(service, account)
	}

	targetPtr, err := syscall.UTF16PtrFromString(targetName(service, account))
	if err != nil {
		return err
	}
	userPtr, err := syscall.UTF16PtrFromString(account)
	if err != nil {
		return err
	}
	blob := []byte(secret)
	cred := credentialW{
		Type:               credTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           userPtr,
	}

	r1, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if r1 == 0 {
		return fmt.Errorf("wincred set %s/%s: %w", service, account, callErr)
	}
	return nil
}

func (s *windowsStore) Delete(service, account string) error {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return errors.New("service and account are required")
	}

	targetPtr, err := syscall.UTF16PtrFromString(targetName(service, account))
	if err != nil {
		return err
	}

	r1, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(credTypeGeneric),
		0,
	)
	if r1 == 0 && !errors.Is(callErr, winErrNotFound) {
		return fmt.Errorf("wincred delete %s/%s: %w", service, account, callErr)
	}
	return nil
}
