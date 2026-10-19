//go:build !darwin && !linux && !windows

package keychain

import (
	"errors"
	"runtime"
)

func load(string) (Credentials, error) {
	return Credentials{}, errors.New("no credential store support for " + runtime.GOOS)
}
