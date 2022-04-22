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
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type ServerAuthCredentials struct {
	credentials.TransportCredentials
}

func (*ServerAuthCredentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	uid, gid, err := readCreds(conn)

	return conn, &ServerAuthInfo{userId: uid, groupId: gid}, err
}

type ServerAuthInfo struct {
	credentials.AuthInfo

	userId  uint32
	groupId uint32
}

func (s *ServerAuthInfo) AuthType() string {
	return fmt.Sprintf("UID: %d GID: %d", s.userId, s.groupId)
}

func GetAuthInfo(ctx context.Context) (uint32, uint32, error) {
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return 0, 0, fmt.Errorf("peer information does not exist for context")
	}

	auth, ok := peer.AuthInfo.(*ServerAuthInfo)
	if !ok {
		return 0, 0, fmt.Errorf("auth info conversion failed")
	}

	return auth.userId, auth.groupId, nil
}
