package keychain

import (
	"fmt"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	procCredRead = advapi32.NewProc("CredReadW")
	procCredFree = advapi32.NewProc("CredFree")
	procCredDel  = advapi32.NewProc("CredDeleteW")
)

const credTypeGeneric = 1

// credential mirrors CREDENTIALW.
type credential struct {
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

// load reads a generic credential from the Windows Credential Manager, where every target
// name is unique: an account is kept under "service:account", or under service itself when
// its user name is that account. Create one with:
//
//	cmdkey /generic:spbstu:LOGIN@edu.spbstu.ru /user:LOGIN@edu.spbstu.ru /pass
func load(service, account string) (Credentials, error) {
	if account != "" {
		if c, err := read(service + ":" + account); err == nil {
			return c, nil
		}
	}
	return read(service)
}

func read(name string) (Credentials, error) {
	target, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return Credentials{}, err
	}
	var cred *credential
	ok, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)))
	if ok == 0 {
		return Credentials{}, fmt.Errorf("credential %q not found in the Credential Manager: %w", name, callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	return Credentials{Account: utf16PtrToString(cred.UserName), Password: blobToString(cred.CredentialBlob, cred.CredentialBlobSize)}, nil
}

func save(string, string, string) error { return ErrUnsupported }

func askPassword(string) (string, error) { return "", ErrUnsupported }

func remove(service, account string) error {
	for _, name := range []string{service + ":" + account, service} {
		if c, err := read(name); err != nil || !strings.EqualFold(c.Account, account) {
			continue
		}
		target, err := syscall.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		if ok, _, callErr := procCredDel.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0); ok == 0 {
			return fmt.Errorf("delete credential %q: %w", name, callErr)
		}
		return nil
	}
	return fmt.Errorf("no credential of %s under %q", account, service)
}

func storeCommand(service, account string) string {
	return fmt.Sprintf("cmdkey /generic:%s:%s /user:%s /pass", service, account, account)
}

// blobToString decodes a credential blob; cmdkey and the Credential Manager store UTF-16LE.
func blobToString(blob *byte, size uint32) string {
	if blob == nil || size == 0 {
		return ""
	}
	b := unsafe.Slice(blob, size)
	if size%2 != 0 {
		return string(b)
	}
	u := make([]uint16, size/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var u []uint16
	for ptr := unsafe.Pointer(p); ; ptr = unsafe.Add(ptr, 2) {
		c := *(*uint16)(ptr)
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
