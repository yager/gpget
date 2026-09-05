//go:build unix && !linux

package main

import "syscall"

func dupTo(oldfd, newfd int) error {
	return syscall.Dup2(oldfd, newfd)
}
