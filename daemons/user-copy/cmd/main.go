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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	zapcr "sigs.k8s.io/controller-runtime/pkg/log/zap"

	dwsv1alpha2 "github.com/DataWorkflowServices/dws/api/v1alpha2"
	"github.com/NearNodeFlash/nnf-dm/daemons/user-copy/pkg/driver"
	userHttp "github.com/NearNodeFlash/nnf-dm/daemons/user-copy/pkg/server"
	nnfv1alpha3 "github.com/NearNodeFlash/nnf-sos/api/v1alpha3"
)

var (
	scheme     = runtime.NewScheme()
	rabbitName string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nnfv1alpha3.AddToScheme(scheme))
	utilruntime.Must(dwsv1alpha2.AddToScheme(scheme))
}

func setupLogAndClient() (logr.Logger, client.Client) {
	encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	zaplogger := zapcr.New(zapcr.Encoder(encoder), zapcr.UseDevMode(true))
	ctrl.SetLogger(zaplogger)

	// controllerruntime logger.
	crLog := ctrl.Log.WithName("user-copy")

	config := ctrl.GetConfigOrDie()

	clnt, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		crLog.Error(err, "Unable to create client")
		os.Exit(1)
	}

	return crLog, clnt
}

func clientSanity(crLog logr.Logger, clnt client.Client) {
	// Sanity check the client connection.
	systemConfig := &dwsv1alpha2.SystemConfiguration{}
	if err := clnt.Get(context.TODO(), types.NamespacedName{Name: "default", Namespace: corev1.NamespaceDefault}, systemConfig); err != nil {
		crLog.Error(err, "Failed to retrieve system config")
		os.Exit(1)
	}
	slog.Info("Found system config", "UID", systemConfig.UID)
}

func main() {
	port := "8080"
	setupOnly := false

	flag.StringVar(&port, "port", port, "Port for server.")
	flag.BoolVar(&setupOnly, "setup-only", setupOnly, "Stop after setup.")

	flag.Parse()

	rabbitName = os.Getenv("NNF_NODE_NAME")
	if rabbitName == "" {
		fmt.Println("Did not find NNF_NODE_NAME")
		os.Exit(1)
	}
	slog.Info("Ready", "node", rabbitName)

	crLog, clnt := setupLogAndClient()
	clientSanity(crLog, clnt)

	// Make one of these for this server, and use it in all requests.
	drvr := &driver.Driver{Client: clnt, Log: crLog, RabbitName: rabbitName}
	httpHandler := &userHttp.UserHttp{Log: crLog, Drvr: drvr, SetupOnly: setupOnly}

	http.HandleFunc("/hello", httpHandler.Hello)
	http.HandleFunc("/trial", httpHandler.TrialRequest)
	http.HandleFunc("/cancel/", httpHandler.CancelRequest)
	http.HandleFunc("/list", httpHandler.ListRequests)

	err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if errors.Is(err, http.ErrServerClosed) {
		slog.Info("the server is closed")
	} else if err != nil {
		slog.Error("unable to start server", "err", err.Error())
	}
}
