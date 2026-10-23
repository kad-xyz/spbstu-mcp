//go:build !windows

package state

func busy(error) bool { return false }
