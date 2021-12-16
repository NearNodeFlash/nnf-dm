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
