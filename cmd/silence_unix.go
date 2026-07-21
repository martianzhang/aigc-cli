//go:build darwin || linux || freebsd

package cmd

import "syscall"

var savedStderr int = -1

func silenceCAPI() {
	savedStderr, _ = syscall.Dup(2)
	nullFd, _ := syscall.Open("/dev/null", syscall.O_WRONLY, 0)
	if nullFd >= 0 {
		syscall.Dup2(nullFd, 2)
		syscall.Close(nullFd)
	}
}

func loudCAPI() {
	if savedStderr >= 0 {
		syscall.Dup2(savedStderr, 2)
		syscall.Close(savedStderr)
		savedStderr = -1
	}
}
