package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"os"
	"syscall"
	"unsafe"

	"google.golang.org/grpc"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"
)

var (
	client pb.RsyncDataMoverClient
)

//export OpenConnection
func OpenConnection(socketAddr *C.char) (conn uintptr, rc int32) {
	connection, err := grpc.Dial("unix://"+C.GoString(socketAddr), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return 0, -1
	}

	client = pb.NewRsyncDataMoverClient(connection)
	if client == nil {
		return 0, int32(syscall.ENOMEM)
	}

	return uintptr(unsafe.Pointer(connection)), 0
}

//export CloseConnection
func CloseConnection(connection uintptr) (rc int32) {

	client = nil

	conn := (*grpc.ClientConn)(unsafe.Pointer(connection))
	if err := conn.Close(); err != nil {
		return -1
	}

	return 0
}

//export Create
func Create(source, destination *C.char) (uid *C.char, rc int32) {
	if client == nil {
		return nil, int32(syscall.EBADR)
	}

	rsp, err := client.Create(context.TODO(), &pb.RsyncDataMovementCreateRequest{
		Source:      C.GoString(source),
		Destination: C.GoString(destination),
		Initiator:   os.Getenv("NODE_NAME"),
		Target:      os.Getenv("NNF_NODE_NAME"),
		Workflow:    os.Getenv("DW_WORKFLOW_NAME"),
		Namespace:   os.Getenv("DW_WORKFLOW_NAMESPACE"),
	})

	if err != nil {
		return nil, int32(syscall.EIO)
	}

	return C.CString(rsp.GetUid()), 0
}

//export Status
func Status(uid *C.char) (state int32, status int32, rc int32) {
	if client == nil {
		return -1, -1, int32(syscall.EBADR)
	}

	rsp, err := client.Status(context.TODO(), &pb.RsyncDataMovementStatusRequest{
		Uid: C.GoString(uid),
	})

	if err != nil {
		return -1, -1, int32(syscall.EIO)
	}

	return int32(rsp.GetState()), int32(rsp.GetStatus()), 0
}

//export Delete
func Delete(uid *C.char) (status int32, rc int32) {
	if client == nil {
		return -1, int32(syscall.EBADR)
	}

	rsp, err := client.Delete(context.TODO(), &pb.RsyncDataMovementDeleteRequest{
		Uid: C.GoString(uid),
	})

	if err != nil {
		return -1, int32(syscall.EIO)
	}

	return int32(rsp.GetStatus()), 0
}

//export Free
func Free(uid *C.char) {
	C.free(unsafe.Pointer(uid))
}

func main() {}
