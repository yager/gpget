//go:build !darwin || !cgo

package notify

func native(_, _, _ string) bool { return false }
