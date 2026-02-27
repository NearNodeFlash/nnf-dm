//go:build darwin

/*
 * Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

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
