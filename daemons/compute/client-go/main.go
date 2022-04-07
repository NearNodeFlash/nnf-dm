package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"

	pb "github.hpe.com/hpe/hpc-rabsw-nnf-dm/daemons/compute/client-go/api"
)

func main() {

	hostname, err := os.Hostname()
	if err != nil || len(hostname) == 0 {
		hostname = os.Getenv("NODE_NAME")
	}
	nodename := flag.String("nodename", hostname, "name of this node")
	workflow := flag.String("workflow", os.Getenv("DW_WORKFLOW_NAME"), "parent workflow name")
	namespace := flag.String("namespace", os.Getenv("DW_WORKFLOW_NAMESPACE"), "parent workflow namespace")
	source := flag.String("source", "", "source file or directory")
	destination := flag.String("destination", "", "destination file or directory")
	dryrun := flag.Bool("dryrun", false, "perfrom dry run of operation")
	skipDelete := flag.Bool("skip-delete", false, "skip deleting the resource after completion")
	socket := flag.String("socket", "/var/run/nnf-dm.sock", "socket address")

	flag.Parse()

	if len(*nodename) == 0 {
		log.Printf("node name is required")
		os.Exit(1)
	}
	if len(*workflow) == 0 {
		log.Printf("workflow name required")
		os.Exit(1)
	}
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

	uid, err := createRequest(ctx, c, *nodename, *workflow, *namespace, *source, *destination, *dryrun)
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

	// Permit skipping the delete step for testing the NNF Data Movement Workflow. This simulates the condition where
	// the user creates a data movement request but fails to delete the request after it is complete (i.e. software crashed
	// and couldn't recover the data movement request). The NNF Data Movement Workflow ensures that these requests are
	// deleted.
	if !*skipDelete {
		log.Printf("Deleting request: %s", uid)
		status, err := deleteRequest(ctx, c, uid)
		if err != nil {
			log.Fatalf("could not delete data movement request: %v", err)
		}
		log.Printf("Data movement request deleted: %s %s", uid, status.String())
	}
}

func createRequest(ctx context.Context, client pb.RsyncDataMoverClient, nodename, workflow, namespace, source, destination string, dryrun bool) (string, error) {

	rsp, err := client.Create(ctx, &pb.RsyncDataMovementCreateRequest{
		Initiator:   nodename,
		Workflow:    workflow,
		Namespace:   namespace,
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

func deleteRequest(ctx context.Context, client pb.RsyncDataMoverClient, uid string) (pb.RsyncDataMovementDeleteResponse_Status, error) {
	rsp, err := client.Delete(ctx, &pb.RsyncDataMovementDeleteRequest{
		Uid: uid,
	})

	if err != nil {
		return pb.RsyncDataMovementDeleteResponse_UNKNOWN, err
	}

	return rsp.GetStatus(), nil
}
