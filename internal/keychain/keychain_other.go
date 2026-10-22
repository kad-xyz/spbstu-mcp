//go:build !darwin && !linux && !windows

package keychain

import (
	"errors"
	"runtime"
)

var errNoStore = errors.New("no credential store support for " + runtime.GOOS)

func load(string, string) (Credentials, error) { return Credentials{}, errNoStore }

func save(string, string, string) error { return errNoStore }

func remove(string, string) error { return errNoStore }

func askPassword(string) (string, error) { return "", errNoStore }

func storeCommand(string, string) string { return "" }
