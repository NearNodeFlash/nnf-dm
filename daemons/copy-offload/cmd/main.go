/*
 * Copyright 2024-2025 Hewlett Packard Enterprise Development LP
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
	"crypto/tls"
	"encoding/pem"
	"errors"
	"flag"
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

	dwsv1alpha3 "github.com/DataWorkflowServices/dws/api/v1alpha3"
	lusv1beta1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1beta1"
	"github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/driver"
	userHttp "github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/server"
	nnfv1alpha6 "github.com/NearNodeFlash/nnf-sos/api/v1alpha6"
)

var (
	scheme     = runtime.NewScheme()
	rabbitName string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nnfv1alpha6.AddToScheme(scheme))
	utilruntime.Must(dwsv1alpha3.AddToScheme(scheme))
	utilruntime.Must(lusv1beta1.AddToScheme(scheme))
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
	nnfNode := &nnfv1alpha6.NnfNode{}
	if err := clnt.Get(context.TODO(), types.NamespacedName{Name: "nnf-nlc", Namespace: rabbitName}, nnfNode); err != nil {
		crLog.Error(err, "Failed to retrieve my own NnfNode")
		os.Exit(1)
	}
}

func main() {
	mock := false
	skipTls := false
	skipToken := false
	var tlsConfig *tls.Config
	var keyBlock *pem.Block

	addr := flag.String("addr", "localhost:4000", "HTTPS network address")
	certFile := flag.String("cert", "cert.pem", "CA/server certificate PEM file. A self-signed cert.")
	keyFile := flag.String("cakey", "key.pem", "CA key PEM file")
	tokenKeyFile := flag.String("tokenkey", "token_key.pem", "File with PEM key used to sign the token.")
	flag.BoolVar(&skipTls, "skip-tls", skipTls, "Skip setting up TLS.")
	flag.BoolVar(&skipToken, "skip-token", skipToken, "Skip the use of a bearer token.")
	flag.BoolVar(&mock, "mock", mock, "Mock mode for tests; does not use k8s.")
	flag.Parse()

	rabbitName = os.Getenv("NNF_NODE_NAME")
	if rabbitName == "" {
		slog.Error("Did not find NNF_NODE_NAME")
		os.Exit(1)
	}
	if os.Getenv("ENVIRONMENT") == "" {
		// "production" or "kind" or "test"
		slog.Error("Did not find ENVIRONMENT")
		os.Exit(1)
	}

	crLog := setupLog()
	// Make one of these for this server, and use it in all requests.
	drvr := driver.NewDriver(crLog, mock)

	if !skipTls {
		serverTLSCert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			slog.Error("Error loading certificate and key file", "error", err.Error())
			os.Exit(1)
		}

		tlsConfig = &tls.Config{
			MinVersion:               tls.VersionTLS13,
			PreferServerCipherSuites: true,
			Certificates:             []tls.Certificate{serverTLSCert},
		}
	}

	if !skipToken {
		// Read the token's key out of its PEM file and decode it to DER form.
		inKey, err := os.ReadFile(*tokenKeyFile)
		if err != nil {
			slog.Error("unable to read back the key file", "error", err.Error())
			os.Exit(1)
		}
		keyBlock, _ = pem.Decode(inKey)
		if keyBlock == nil {
			slog.Error("unable to decode PEM key for token")
			os.Exit(1)
		}
	}

	if !mock {
		clnt := setupClient(crLog)
		clientSanity(crLog, clnt, rabbitName)
		drvr.Client = clnt
	}

	slog.Info("Ready", "node", rabbitName, "addr", *addr, "mock", mock, "TLS", !skipTls, "token", !skipToken)

	// Make one of these for this server, and use it in all requests.
	httpHandler := &userHttp.UserHttp{
		Log:  crLog,
		Drvr: drvr,
		Mock: mock,
	}
	if !skipToken {
		httpHandler.KeyBytes = keyBlock.Bytes
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", httpHandler.Hello)
	mux.HandleFunc("/trial", httpHandler.TrialRequest)
	mux.HandleFunc("/cancel/", httpHandler.CancelRequest)
	mux.HandleFunc("/list", httpHandler.ListRequests)
	mux.HandleFunc("/status", httpHandler.GetRequest)

	srv := &http.Server{
		Addr:      *addr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	var err error
	if !skipTls {
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		slog.Info("the server is closed")
	} else if err != nil {
		slog.Error("unable to start server", "err", err.Error())
	}
}
