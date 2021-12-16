//go:build linux
// +build linux

package auth

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func readCreds(c net.Conn) (uint32, uint32, error) {

	unixConn, ok := c.(*net.UnixConn)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected socket type")
	}

	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, 0, fmt.Errorf("error opening raw connection: %s", err)
	}

	var cred *unix.Ucred

	// Control method allows one to run a callback function against the raw socket.
	// Use it to execute the system call unix.GetsockoptUcred() to obtain the user
	// credentials.
	raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptUcred(int(fd),
			unix.SOL_SOCKET,
			unix.SO_PEERCRED)
	})

	if err != nil {
		return 0, 0, fmt.Errorf("Get Socketopt Ucred error %s", err)
	}

	return cred.Uid, cred.Gid, nil

}
