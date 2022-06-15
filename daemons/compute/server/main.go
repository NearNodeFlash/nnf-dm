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
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/takama/daemon"
	"google.golang.org/grpc"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/api"

	"github.com/NearNodeFlash/nnf-dm/daemons/compute/server/auth"
	server "github.com/NearNodeFlash/nnf-dm/daemons/compute/server/servers"
)

const (
	name        = "nnf-dm"
	description = "Near-Node Flash (NNF) Data Movement Service"
	usage       = "Usage nnf-dm install | remove | start | stop | status"
)

type Service struct {
	daemon.Daemon
}

func (service *Service) Manage() (string, error) {

	if len(os.Args) > 1 {
		command := os.Args[1]
		switch command {
		case "install":
			return service.Install(os.Args[2:]...)
		case "remove":
			return service.Remove()
		case "start":
			return service.Start()
		case "stop":
			return service.Stop()
		case "status":
			return service.Status()
		}
	}

	var socketAddr = flag.String("socket", "/var/run/nnf-dm.sock", "path of the NNF data movement socket")

	options, err := server.GetOptions()
	if err != nil {
		return "Failed to get nnf-dm server options", err
	}

	flag.Parse()

	server, err := server.Create(options)
	if err != nil {
		return "Failed to create nnf-dm server", err
	}

	// Set up channel on which to send signal notifications; must use a buffered
	// channel or risk missing the signal if we're not setup to receive the signal
	// when it is sent.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, os.Kill, syscall.SIGTERM)

	// Set up listener to our socket address
	os.Remove(*socketAddr)
	listener, err := net.Listen("unix", *socketAddr)
	if err != nil {
		return fmt.Sprintf("Failed to listen at socket %s", *socketAddr), err
	}

	grpcServer := grpc.NewServer(grpc.Creds(&auth.ServerAuthCredentials{}))
	pb.RegisterDataMoverServer(grpcServer, server)

	go service.Run(grpcServer, listener)

	for {
		select {
		case <-interrupt:
			stdlog.Println("Stop listening at ", listener.Addr())
			listener.Close()
			return "Daemon was killed", nil
		}
	}
}

func (service *Service) Run(server *grpc.Server, listener net.Listener) error {

	stdlog.Println("Start listening at ", listener.Addr())
	if err := server.Serve(listener); err != nil {
		return err
	}

	return nil
}

var stdlog, errlog *log.Logger

func init() {
	stdlog = log.New(os.Stdout, "", log.Ldate|log.Ltime)
	errlog = log.New(os.Stderr, "", log.Ldate|log.Ltime)
}

func main() {

	kindFn := func() daemon.Kind {
		if runtime.GOOS == "darwin" {
			return daemon.UserAgent
		}
		return daemon.SystemDaemon
	}

	d, err := daemon.New(name, description, kindFn(), "network-online.target")
	if err != nil {
		errlog.Println("Error: ", err)
		os.Exit(1)
	}

	service := &Service{d}

	status, err := service.Manage()
	if err != nil {
		errlog.Println(status, "\nError: ", err)
		os.Exit(1)
	}

	fmt.Println(status)
}
