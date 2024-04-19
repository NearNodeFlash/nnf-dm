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
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/client-go/api"
)

func main() {

	workflow := flag.String("workflow", os.Getenv("DW_WORKFLOW_NAME"), "parent workflow name")
	namespace := flag.String("namespace", os.Getenv("DW_WORKFLOW_NAMESPACE"), "parent workflow namespace")
	source := flag.String("source", "", "source file or directory")
	destination := flag.String("destination", "", "destination file or directory")
	dryrun := flag.Bool("dryrun", false, "perform dry run of operation")
	skipDelete := flag.Bool("skip-delete", false, "skip deleting the resource after completion")
	socket := flag.String("socket", "/var/run/nnf-dm.sock", "socket address")
	maxWaitTime := flag.Int64("max-wait-time", 0, "maximum time to wait for status completion, in seconds.")
	count := flag.Int("count", 1, "number of requests to create")
	cancelExpiryTime := flag.Duration("cancel", -time.Second, "duration after create to cancel request")
	dcpOptions := flag.String("dcp-options", "", "extra options to provide to dcp")
	logStdout := flag.Bool("log-stdout", false, "enable server-side logging of stdout on successful dm")
	storeStdout := flag.Bool("store-stdout", false, "store stdout in status message on successful dm")
	slots := flag.Int("slots", -1, "slots to use in mpirun hostfile. -1 defers to system config, 0 omits from hostfile")
	maxSlots := flag.Int("max-slots", -1, "max_slots to use in mpirun hostfile. -1 defers to system config, 0 omits from hostfile")
	profile := flag.String("profile", "", "which data movement profile to use on the server, empty defaults to default profile")

	flag.Parse()

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

	versionResponse, err := versionRequest(ctx, c)
	if err != nil {
		log.Fatalf("could not retrieve version information: %v", err)
	}

	log.Printf("Data Mover Version: %s\n", versionResponse.GetVersion())
	log.Printf("Data Mover Supported API Versions: %v\n", versionResponse.GetApiVersions())

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

	// Create all of the requests at once
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}
	responses := make([]*pb.DataMovementCreateResponse, 0)
	for i := 0; i < *count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			log.Printf("Creating request %d of %d...", i+1, *count)
			createResponse, err := createRequest(ctx, c, *workflow, *namespace,
				*source, *destination, *dryrun, *dcpOptions,
				*logStdout, *storeStdout, *slots, *maxSlots,
				*profile)
			if err != nil {
				log.Fatalf("could not create data movement request: %v", err)
			}

			lock.Lock()
			responses = append(responses, createResponse)
			lock.Unlock()

			if createResponse.GetStatus() != pb.DataMovementCreateResponse_SUCCESS {
				log.Fatalf("create request failed: %+v", createResponse)
			}

			log.Printf("Data movement request created: %s", createResponse.GetUid())
		}(i)
	}

	wg.Wait()

	for _, resp := range responses {
		uid := resp.GetUid()
		wg.Add(1)

		go func(uid string, resp *pb.DataMovementCreateResponse) {
			defer wg.Done()

			// Cancel the data movement after specified amount of time
			if *cancelExpiryTime >= 0 {
				log.Printf("Waiting %s before cancelling request\n", (*cancelExpiryTime).String())
				time.Sleep(*cancelExpiryTime)

				log.Printf("Canceling request: %v", uid)
				cancelResponse, err := cancelRequest(ctx, c, *workflow, *namespace, uid)
				if err != nil {
					log.Fatalf("error initiating data movement cancel request: %v", err)
				}
				log.Printf("Data movement request cancel initiated: %v %v", uid, cancelResponse.String())
			}

			// Poll request to check for completed/cancelled
			for {
				statusResponse, err := getStatus(ctx, c, *workflow, *namespace, uid, *maxWaitTime)
				if err != nil {
					log.Fatalf("failed to get data movement status: %v", err)
				}

				if statusResponse.GetStatus() == pb.DataMovementStatusResponse_FAILED ||
					statusResponse.GetStatus() == pb.DataMovementStatusResponse_INVALID {
					log.Fatalf("Data movement failed: %s", statusString(statusResponse))
				}

				log.Printf("Data movement status: %s", statusString(statusResponse))
				if statusResponse.GetState() == pb.DataMovementStatusResponse_COMPLETED {
					break
				}

				time.Sleep(time.Second)
			}

			log.Printf("Data movement completed: %s", resp.GetUid())
		}(uid, resp)
	}

	wg.Wait()

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
				log.Fatalf("could not delete data movement request: %s", err)
			}

			if deleteResponse.Status == pb.DataMovementDeleteResponse_NOT_FOUND {
				log.Printf("Data movement request deleted (not found): %v %v", uid, deleteResponse.String())
			} else if deleteResponse.Status != pb.DataMovementDeleteResponse_SUCCESS {
				log.Fatalf("data movement delete failed: %v %+v", uid, deleteResponse)
			} else {
				log.Printf("Data movement request deleted: %v %v", uid, deleteResponse.String())
			}
		}

		// Print out the list again to verify deletes
		listResponse, err := listRequests(ctx, c, *workflow, *namespace)
		if err != nil {
			log.Fatalf("could not retrieve list of data movement requests: %v", err)
		}
		log.Printf("List of Data movement requests: %s", strings.Join(listResponse.GetUids(), ", "))
	}
}

func versionRequest(ctx context.Context, client pb.DataMoverClient) (*pb.DataMovementVersionResponse, error) {
	rsp, err := client.Version(ctx, &emptypb.Empty{})

	if err != nil {
		return nil, err
	}

	return rsp, nil
}

func createRequest(ctx context.Context, client pb.DataMoverClient, workflow, namespace,
	source, destination string, dryrun bool, dcpOptions string, logStdout, storeStdout bool,
	slots, maxSlots int, profile string) (*pb.DataMovementCreateResponse, error) {

	rsp, err := client.Create(ctx, &pb.DataMovementCreateRequest{
		Workflow: &pb.DataMovementWorkflow{
			Name:      workflow,
			Namespace: namespace,
		},
		Source:      source,
		Destination: destination,
		Dryrun:      dryrun,
		DcpOptions:  dcpOptions,
		LogStdout:   logStdout,
		StoreStdout: storeStdout,
		Slots:       int32(slots),
		MaxSlots:    int32(maxSlots),
		Profile:     profile,
	})

	if err != nil {
		return nil, err
	}

	return rsp, nil
}

func getStatus(ctx context.Context, client pb.DataMoverClient, workflow string, namespace string, uid string, maxWaitTime int64) (*pb.DataMovementStatusResponse, error) {
	rsp, err := client.Status(ctx, &pb.DataMovementStatusRequest{
		Workflow: &pb.DataMovementWorkflow{
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

func statusString(status *pb.DataMovementStatusResponse) string {
	return fmt.Sprintf("state: %s, status: %s, message: %s, commandStatus: %+v", status.State, status.Status, status.Message, status.CommandStatus)
}

func listRequests(ctx context.Context, client pb.DataMoverClient, workflow string, namespace string) (*pb.DataMovementListResponse, error) {
	rsp, err := client.List(ctx, &pb.DataMovementListRequest{
		Workflow: &pb.DataMovementWorkflow{
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
		Workflow: &pb.DataMovementWorkflow{
			Name:      workflow,
			Namespace: namespace,
		},
		Uid: uid,
	})

	// It's already been deleted if it's not found
	if err != nil && rsp.Status != pb.DataMovementDeleteResponse_NOT_FOUND {
		return nil, err
	}

	return rsp, nil
}

func cancelRequest(ctx context.Context, client pb.DataMoverClient, workflow string, namespace string, uid string) (*pb.DataMovementCancelResponse, error) {
	rsp, err := client.Cancel(ctx, &pb.DataMovementCancelRequest{
		Workflow: &pb.DataMovementWorkflow{
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
