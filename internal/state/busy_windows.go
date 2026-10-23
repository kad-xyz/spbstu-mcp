package state

import (
	"errors"
	"io/fs"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: the file is open in another process.
const errSharingViolation = syscall.Errno(32)

func busy(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, errSharingViolation)
}
