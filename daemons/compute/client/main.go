package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/api"
)

func main() {

	source := flag.String("source", "", "source file or directory")
	destination := flag.String("destination", "", "destination file or directory")
	dryrun := flag.Bool("dryrun", false, "perfrom dry run of operation")
	socket := flag.String("socket", "/var/run/nnf-dm.sock", "socket address")

	flag.Parse()

	if len(*source) == 0 || len(*destination) == 0 {
		log.Printf("source and destination required")
		os.Exit(1)
	}

	socketAddr := "unix:///" + *socket

	log.Printf("Connecting to %s", socketAddr)
	conn, err := grpc.Dial(socketAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := pb.NewRsyncDataMoverClient(conn)

	uid, err := createRequest(ctx, c, *source, *destination, *dryrun)
	if err != nil {
		log.Fatalf("could not create data movement request: %v", err)
	}
	log.Printf("Data movement request created: %s", uid)

	for {
		state, status, err := getStatus(ctx, c, uid)
		if status == pb.RsyncDataMovementStatusResponse_FAILED {
			log.Fatalf("Data movement failed: %v", err)
		}

		log.Printf("Data movement %s", state.String())
		if state == pb.RsyncDataMovementStatusResponse_COMPLETED {
			break
		}

		time.Sleep(time.Second)
	}

	log.Printf("Data movement completed: %s", uid)
}

func createRequest(ctx context.Context, client pb.RsyncDataMoverClient, source string, destination string, dryrun bool) (string, error) {

	rsp, err := client.Create(ctx, &pb.RsyncDataMovementCreateRequest{
		Source:      source,
		Destination: destination,
		Dryrun:      dryrun,
	})

	if err != nil {
		return "", err
	}

	return rsp.GetUid(), nil
}

func getStatus(ctx context.Context, client pb.RsyncDataMoverClient, uid string) (pb.RsyncDataMovementStatusResponse_State, pb.RsyncDataMovementStatusResponse_Status, error) {
	rsp, err := client.Status(ctx, &pb.RsyncDataMovementStatusRequest{
		Uid: uid,
	})

	if err != nil {
		return pb.RsyncDataMovementStatusResponse_UNKNOWN_STATE, pb.RsyncDataMovementStatusResponse_FAILED, err
	}

	return rsp.GetState(), rsp.GetStatus(), nil
}
