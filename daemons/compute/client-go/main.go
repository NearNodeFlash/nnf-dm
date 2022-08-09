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
	"strings"
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
	maxWaitTime := flag.Int64("max-wait-time", 0, "maximum time to wait for status completion, in seconds.")
	count := flag.Int("count", 1, "number of requests to create")

	flag.Parse()

	if len(*workflow) == 0 {
		log.Printf("workflow name required")
		os.Exit(1)
	}
	if len(*source) == 0 || len(*destination) == 0 {
		log.Printf("source and destination required")
		os.Exit(1)
	}
	if *count <= 0 {
		log.Printf("count must be >= 1")
		os.Exit(1)
	}

	socketAddr := "unix://" + *socket

	log.Printf("Connecting to %s", socketAddr)
	conn, err := grpc.Dial(socketAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := pb.NewDataMoverClient(conn)

	for i := 0; i < *count; i++ {
		log.Printf("Creating request %d of %d...", i+1, *count)

		createResponse, err := createRequest(ctx, c, *workflow, *namespace, *source, *destination, *dryrun)
		if err != nil {
			log.Fatalf("could not create data movement request: %v", err)
		}

		if createResponse.GetStatus() == pb.DataMovementCreateResponse_SUCCESS {
			log.Printf("Data movement request created: %s", createResponse.GetUid())
		} else {
			log.Fatal("Create request failed: ", createResponse.String())
		}

		// Wait for request to be completed
		for {
			statusResponse, err := getStatus(ctx, c, *workflow, *namespace, createResponse.GetUid(), *maxWaitTime)
			if statusResponse.GetStatus() == pb.DataMovementStatusResponse_FAILED {
				log.Fatalf("Data movement failed: %v", err)
			}

			log.Printf("Data movement status %s", statusResponse.String())
			if statusResponse.GetState() == pb.DataMovementStatusResponse_COMPLETED {
				break
			}

			time.Sleep(time.Second)
		}

		log.Printf("Data movement completed: %s", createResponse.GetUid())
	}

	// Get the list of Data Movement Requests
	listResponse, err := listRequests(ctx, c, *workflow, *namespace)
	if err != nil {
		log.Fatalf("could not retrieve list of data movement requests: %v", err)
	}
	log.Printf("List of Data movement requests: %s", strings.Join(listResponse.GetUids(), ", "))

	// Permit skipping the delete step for testing the NNF Data Movement Workflow. This simulates the condition where
	// the user creates a data movement request but fails to delete the request after it is complete (i.e. software crashed
	// and couldn't recover the data movement request). The NNF Data Movement Workflow ensures that these requests are
	// deleted.
	if !*skipDelete {
		// Use List to cleanup and delete requests
		for _, uid := range listResponse.GetUids() {
			log.Printf("Deleting request: %v", uid)
			deleteResponse, err := deleteRequest(ctx, c, *workflow, *namespace, uid)
			if err != nil {
				log.Fatalf("could not delete data movement request: %v", err)
			}
			log.Printf("Data movement request deleted: %v %v", uid, deleteResponse.String())
		}

		// Print out the list again to verify deletes
		listResponse, err := listRequests(ctx, c, *workflow, *namespace)
		if err != nil {
			log.Fatalf("could not retrieve list of data movement requests: %v", err)
		}
		log.Printf("List of Data movement requests: %s", strings.Join(listResponse.GetUids(), ", "))
	}
}

func createRequest(ctx context.Context, client pb.DataMoverClient, workflow, namespace, source, destination string, dryrun bool) (*pb.DataMovementCreateResponse, error) {

	rsp, err := client.Create(ctx, &pb.DataMovementCreateRequest{
		Workflow: &pb.DataMovementCreateRequest_Workflow{
			Name:      workflow,
			Namespace: namespace,
		},
		Source:      source,
		Destination: destination,
		Dryrun:      dryrun,
	})

	if err != nil {
		return nil, err
	}

	return rsp, nil
}

func getStatus(ctx context.Context, client pb.DataMoverClient, workflow string, namespace string, uid string, maxWaitTime int64) (*pb.DataMovementStatusResponse, error) {
	rsp, err := client.Status(ctx, &pb.DataMovementStatusRequest{
		Workflow: &pb.DataMovementCreateRequest_Workflow{
			Name:      workflow,
			Namespace: namespace,
		},
		Uid:         uid,
		MaxWaitTime: maxWaitTime,
	})

	if err != nil {
		return nil, err
	}

	return rsp, nil
}

func listRequests(ctx context.Context, client pb.DataMoverClient, workflow string, namespace string) (*pb.DataMovementListResponse, error) {
	rsp, err := client.List(ctx, &pb.DataMovementListRequest{
		Workflow: &pb.DataMovementCreateRequest_Workflow{
			Name:      workflow,
			Namespace: namespace,
		},
	})

	if err != nil {
		return nil, err
	}

	return rsp, err
}

func deleteRequest(ctx context.Context, client pb.DataMoverClient, workflow string, namespace string, uid string) (*pb.DataMovementDeleteResponse, error) {
	rsp, err := client.Delete(ctx, &pb.DataMovementDeleteRequest{
		Workflow: &pb.DataMovementCreateRequest_Workflow{
			Name:      workflow,
			Namespace: namespace,
		},
		Uid: uid,
	})

	if err != nil {
		return nil, err
	}

	return rsp, nil
}
