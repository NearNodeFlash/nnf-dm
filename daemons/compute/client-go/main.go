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

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/client-go/api"
)

func main() {

	workflow := flag.String("workflow", os.Getenv("DW_WORKFLOW_NAME"), "parent workflow name")
	namespace := flag.String("namespace", os.Getenv("DW_WORKFLOW_NAMESPACE"), "parent workflow namespace")
	source := flag.String("source", "", "source file or directory")
	destination := flag.String("destination", "", "destination file or directory")
	dryrun := flag.Bool("dryrun", false, "perfrom dry run of operation")
	skipDelete := flag.Bool("skip-delete", false, "skip deleting the resource after completion")
	socket := flag.String("socket", "/var/run/nnf-dm.sock", "socket address")

	flag.Parse()

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

	c := pb.NewDataMoverClient(conn)

	uid, status, err := createRequest(ctx, c, *workflow, *namespace, *source, *destination, *dryrun)
	if err != nil {
		log.Fatalf("could not create data movement request: %v", err)
	}

	if status == pb.DataMovementCreateResponse_CREATED {
		log.Printf("Data movement request created: %s", uid)
	} else {
		log.Fatal("Create request failed")
	}

	for {
		state, status, err := getStatus(ctx, c, uid)
		if status == pb.DataMovementStatusResponse_FAILED {
			log.Fatalf("Data movement failed: %v", err)
		}

		log.Printf("Data movement %s", state.String())
		if state == pb.DataMovementStatusResponse_COMPLETED {
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

func createRequest(ctx context.Context, client pb.DataMoverClient, workflow, namespace, source, destination string, dryrun bool) (string, pb.DataMovementCreateResponse_Status, error) {

	rsp, err := client.Create(ctx, &pb.DataMovementCreateRequest{
		Workflow:    workflow,
		Namespace:   namespace,
		Source:      source,
		Destination: destination,
		Dryrun:      dryrun,
	})

	if err != nil {
		return "", pb.DataMovementCreateResponse_FAILED, err
	}

	return rsp.GetUid(), rsp.GetStatus(), nil
}

func getStatus(ctx context.Context, client pb.DataMoverClient, uid string) (pb.DataMovementStatusResponse_State, pb.DataMovementStatusResponse_Status, error) {
	rsp, err := client.Status(ctx, &pb.DataMovementStatusRequest{
		Uid: uid,
	})

	if err != nil {
		return pb.DataMovementStatusResponse_UNKNOWN_STATE, pb.DataMovementStatusResponse_FAILED, err
	}

	return rsp.GetState(), rsp.GetStatus(), nil
}

func deleteRequest(ctx context.Context, client pb.DataMoverClient, uid string) (pb.DataMovementDeleteResponse_Status, error) {
	rsp, err := client.Delete(ctx, &pb.DataMovementDeleteRequest{
		Uid: uid,
	})

	if err != nil {
		return pb.DataMovementDeleteResponse_UNKNOWN, err
	}

	return rsp.GetStatus(), nil
}
