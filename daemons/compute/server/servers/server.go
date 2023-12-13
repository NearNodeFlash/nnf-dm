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

package server

import (
	"flag"
	"os"

	pb "github.com/NearNodeFlash/nnf-dm/daemons/compute/client-go/api"
)

type ServerOptions struct {
	host      string
	port      string
	tokenFile string
	certFile  string

	name       string
	nodeName   string
	sysConfig  string
	CpuProfile string
	simulated  bool

	k8sQPS   int
	k8sBurst int
}

func GetOptions() (*ServerOptions, error) {
	opts := ServerOptions{
		host:       os.Getenv("KUBERNETES_SERVICE_HOST"),
		port:       os.Getenv("KUBERNETES_SERVICE_PORT"),
		name:       os.Getenv("NODE_NAME"),
		nodeName:   os.Getenv("NNF_NODE_NAME"),
		tokenFile:  os.Getenv("NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE"),
		certFile:   os.Getenv("NNF_DATA_MOVEMENT_SERVICE_CERT_FILE"),
		CpuProfile: os.Getenv("NNF_DATA_MOVEMENT_SERVICE_CPU_PROFILE"),
		simulated:  false,

		// These options adjust the client-side rate-limiting for k8s. The new defaults are 50 and
		// 100 (rather than 5, 10). See more info https://github.com/kubernetes/kubernetes/pull/116121
		// According to that PR, it appears that client-side rate-limiting is going away.
		k8sQPS:   50,
		k8sBurst: 100,
	}

	flag.StringVar(&opts.host, "kubernetes-service-host", opts.host, "Kubernetes service host address")
	flag.StringVar(&opts.port, "kubernetes-service-port", opts.port, "Kubernetes service port number")
	flag.StringVar(&opts.name, "node-name", opts.name, "Name of this compute resource")
	flag.StringVar(&opts.nodeName, "nnf-node-name", opts.nodeName, "NNF node name that should handle the data movement request")
	flag.StringVar(&opts.tokenFile, "service-token-file", opts.tokenFile, "Path to the NNF data movement service token")
	flag.StringVar(&opts.certFile, "service-cert-file", opts.certFile, "Path to the NNF data movement service certificate")
	flag.StringVar(&opts.sysConfig, "sys-config", "default", "Name of the system configuration containing this compute resource")
	flag.BoolVar(&opts.simulated, "simulated", opts.simulated, "Run in simulation mode where no requests are sent to the server")
	flag.IntVar(&opts.k8sQPS, "kubernetes-qps", opts.k8sQPS, "Kubernetes client queries per second (QPS)")
	flag.IntVar(&opts.k8sBurst, "kubernetes-burst", opts.k8sBurst, "Kubernetes client additional concurrent calls above QPS")
	flag.StringVar(&opts.CpuProfile, "cpu-profile", opts.CpuProfile,
		"Enable and dump CPU profiling data to this file after daemon is stopped. Timestamp is added to end of filename.")
	flag.Parse()
	return &opts, nil
}

type Server interface {
	pb.DataMoverServer

	StartManager() error
}

func Create(opts *ServerOptions) (Server, error) {
	if opts.simulated {
		return CreateSimulatedServer(opts)
	}

	return CreateDefaultServer(opts)
}
