//go:build !darwin || !cgo

package notify

// Settings has nothing to report off macOS.
func Settings() string { return "" }
