/*
 * Copyright 2024 Hewlett Packard Enterprise Development LP
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
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	zapcr "sigs.k8s.io/controller-runtime/pkg/log/zap"

	dwsv1alpha2 "github.com/DataWorkflowServices/dws/api/v1alpha2"
	"github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/driver"
	userHttp "github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/server"
	nnfv1alpha4 "github.com/NearNodeFlash/nnf-sos/api/v1alpha4"
)

var (
	scheme     = runtime.NewScheme()
	rabbitName string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nnfv1alpha4.AddToScheme(scheme))
	utilruntime.Must(dwsv1alpha2.AddToScheme(scheme))
}

func setupLog() logr.Logger {
	encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	zaplogger := zapcr.New(zapcr.Encoder(encoder), zapcr.UseDevMode(true))
	ctrl.SetLogger(zaplogger)

	// controllerruntime logger.
	crLog := ctrl.Log.WithName("copy-offload")
	return crLog
}

func setupClient(crLog logr.Logger) client.Client {
	config := ctrl.GetConfigOrDie()

	clnt, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		crLog.Error(err, "Unable to create client")
		os.Exit(1)
	}
	return clnt
}

func clientSanity(crLog logr.Logger, clnt client.Client, rabbitName string) {
	// Sanity check the client connection.
	nnfNode := &nnfv1alpha4.NnfNode{}
	if err := clnt.Get(context.TODO(), types.NamespacedName{Name: "nnf-nlc", Namespace: rabbitName}, nnfNode); err != nil {
		crLog.Error(err, "Failed to retrieve my own NnfNode")
		os.Exit(1)
	}
}

func main() {
	mock := false

	addr := flag.String("addr", "localhost:4000", "HTTPS network address")
	//certFile := flag.String("cert", "cert.pem", "server certificate PEM file")
	//keyFile := flag.String("key", "key.pem", "server key PEM file")
	//clientCertFile := flag.String("clientcert", "clientcert.pem", "client certificate PEM file")
	flag.BoolVar(&mock, "mock", mock, "Mock mode for tests; does not use k8s.")
	flag.Parse()

	rabbitName = os.Getenv("NNF_NODE_NAME")
	if rabbitName == "" {
		fmt.Println("Did not find NNF_NODE_NAME")
		os.Exit(1)
	}

	crLog := setupLog()
	// Make one of these for this server, and use it in all requests.
	drvr := &driver.Driver{Log: crLog, RabbitName: rabbitName, Mock: mock}
	if !mock {
		clnt := setupClient(crLog)
		clientSanity(crLog, clnt, rabbitName)
		drvr.Client = clnt
	}

	slog.Info("Ready", "node", rabbitName, "addr", *addr, "mock", mock)

	httpHandler := &userHttp.UserHttp{Log: crLog, Drvr: drvr, Mock: mock}

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", httpHandler.Hello)
	mux.HandleFunc("/trial", httpHandler.TrialRequest)
	mux.HandleFunc("/cancel/", httpHandler.CancelRequest)
	mux.HandleFunc("/list", httpHandler.ListRequests)

	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		slog.Info("the server is closed")
	} else if err != nil {
		slog.Error("unable to start server", "err", err.Error())
	}
}
