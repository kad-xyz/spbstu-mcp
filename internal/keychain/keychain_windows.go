package keychain

import (
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	procCredRead = advapi32.NewProc("CredReadW")
	procCredFree = advapi32.NewProc("CredFree")
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

// load reads the generic credential named service from the Windows Credential Manager.
// Create it with:
//
//	cmdkey /generic:spbstu /user:LOGIN@edu.spbstu.ru /pass
func load(service string) (Credentials, error) {
	target, err := syscall.UTF16PtrFromString(service)
	if err != nil {
		return Credentials{}, err
	}
	var cred *credential
	ok, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)))
	if ok == 0 {
		return Credentials{}, fmt.Errorf("credential %q not found in the Credential Manager: %w", service, callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	return Credentials{Account: utf16PtrToString(cred.UserName), Password: blobToString(cred.CredentialBlob, cred.CredentialBlobSize)}, nil
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
