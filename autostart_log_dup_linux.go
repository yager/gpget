//go:build linux

package main

import "syscall"

// linux/arm64 has no dup2 syscall; Dup3 with flags=0 is the same operation.
func dupTo(oldfd, newfd int) error {
	return syscall.Dup3(oldfd, newfd, 0)
}
