//go:build darwin
// +build darwin

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
		return 0, 0, fmt.Errorf("error opening raw connection: %v", err)
	}

	var cred *unix.Xucred

	raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptXucred(int(fd),
			unix.SOL_LOCAL,
			unix.LOCAL_PEERCRED)
	})

	if err != nil {
		return 0, 0, fmt.Errorf("Get Socketopt Xucred error %s", err)
	}

	return cred.Uid, cred.Groups[0], nil
}
