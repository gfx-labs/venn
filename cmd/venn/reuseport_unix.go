//go:build unix

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func reusePort(network, address string, conn syscall.RawConn) error {
	return conn.Control(func(descriptor uintptr) {
		syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
}
