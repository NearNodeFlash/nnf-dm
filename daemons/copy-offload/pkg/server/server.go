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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/driver"
	nnfv1alpha9 "github.com/NearNodeFlash/nnf-sos/api/v1alpha9"
	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
)

// UserHttp will have only one instance per process, shared by all threads
// in the process.
type UserHttp struct {
	Log      logr.Logger
	Drvr     *driver.Driver
	InTest   bool
	Mock     bool
	KeyBytes []byte
	Server   *http.Server
}

// The signing algorithm that we expect was used when signing the JWT.
var jwtSigningAlgorithm = jwt.SigningMethodHS256

func (user *UserHttp) verifyToken(tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwtSigningAlgorithm.Name {
			return nil, errors.New("unexpected signing method")
		}
		return user.KeyBytes, nil
	})
	if err != nil {
		return fmt.Errorf("token parse failed: %w", err)
	}
	if !token.Valid {
		return fmt.Errorf("invalid token")
	}
	return nil
}

func (user *UserHttp) validateMessage(w http.ResponseWriter, req *http.Request) string {
	// Validate the bearer token, if the server is using one.
	if len(user.KeyBytes) > 0 {
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return ""
		}
		tokenString := strings.TrimSpace(strings.Replace(authHeader, "Bearer", "", 1))
		if err := user.verifyToken(tokenString); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return ""
		}
	}

	// See COPY_OFFLOAD_API_VERSION in copy-offload.h.
	// This applies to the format of the request sent by the client as well
	// as the format of our response.
	apiVersion := req.Header.Get("Accepts-version")
	if apiVersion != "1.0" {
		// The RFC says Not Accceptable should return a list of valid versions.
		http.Error(w, "Valid versions: 1.0", http.StatusNotAcceptable)
		return ""
	}
	return apiVersion
}

func (user *UserHttp) Hello(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "GET" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = user.validateMessage(w, req); apiVersion == "" {
		return
	}

	user.Log.Info("Hello")
	fmt.Fprintf(w, "hello back at ya\n")
}

func (user *UserHttp) GetRequest(w http.ResponseWriter, req *http.Request) {
	var err error
	var apiVersion string
	if req.Method != "GET" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = user.validateMessage(w, req); apiVersion == "" {
		return
	}

	user.Log.Info("In GetRequest", "version", apiVersion, "url", req.URL)
	urlParts, err := url.Parse(req.URL.String())
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to parse URL query parameters: %s", err.Error()), http.StatusBadRequest)
		return
	}
	requestName := filepath.Base(urlParts.Path)
	params := urlParts.Query()

	// This is the v1.0 apiVersion input. See COPY_OFFLOAD_API_VERSION.
	var statreq driver.StatusRequest
	statreq.RequestName = requestName
	statreq.MaxWaitSecs, err = strconv.Atoi(params.Get("maxWaitSecs"))
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to parse maxWaitSecs: %s", err.Error()), http.StatusBadRequest)
		return
	}
	params.Del("maxWaitSecs")
	if params.Encode() != "" {
		http.Error(w, fmt.Sprintf("unexpected query parameters: %s", params.Encode()), http.StatusBadRequest)
		return
	}
	if err := statreq.Validator(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user.Log.Info("  GetRequest", "dmreq", statreq)

	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	var http_code int
	// This is the v1.0 apiVersion output. See COPY_OFFLOAD_API_VERSION.
	var response *driver.DataMovementStatusResponse_v1_0
	if user.Mock {
		response, http_code, err = drvrReq.GetRequestMock(context.TODO(), statreq)
	} else {
		response, http_code, err = drvrReq.GetRequest(context.TODO(), statreq)
	}
	if err != nil {
		if http_code > 0 {
			http.Error(w, err.Error(), http_code)
		} else {
			http.Error(w, fmt.Sprintf("unable to get request: %s\n", err.Error()), http.StatusInternalServerError)
		}
		return
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("unable to encode data movement status response: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	// StatusOK is implied.
}

func (user *UserHttp) GetActiveRequests() ([]string, error) {
	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	return drvrReq.ListRequests(context.TODO())
}

func (user *UserHttp) ListRequests(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "GET" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = user.validateMessage(w, req); apiVersion == "" {
		return
	}

	items, err := user.GetActiveRequests()
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to list requests: %s\n", err.Error()), http.StatusInternalServerError)
		return
	}
	// This is the v1.0 apiVersion output. See COPY_OFFLOAD_API_VERSION.
	if len(items) > 0 {
		fmt.Fprintln(w, strings.Join(items, ","))
	}
}

func (user *UserHttp) CancelRequest(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "DELETE" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = user.validateMessage(w, req); apiVersion == "" {
		return
	}
	user.Log.Info("In DELETE", "version", apiVersion, "url", req.URL)
	urlParts, err := url.Parse(req.URL.String())
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to parse URL: %s", err.Error()), http.StatusBadRequest)
		return
	}
	name := filepath.Base(urlParts.Path)

	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	if err := drvrReq.CancelRequest(context.TODO(), name); err != nil {
		http.Error(w, fmt.Sprintf("unable to cancel request: %s", err.Error()), http.StatusNotFound)
		return
	}
	// StatusOK is implied.
}

func (user *UserHttp) TrialRequest(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "POST" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = user.validateMessage(w, req); apiVersion == "" {
		return
	}
	user.Log.Info("In TrialRequest", "version", apiVersion, "url", req.URL)

	// This is the v1.0 apiVersion input. See COPY_OFFLOAD_API_VERSION.
	var dmreq driver.DMRequest
	if err := json.NewDecoder(req.Body).Decode(&dmreq); err != nil {
		http.Error(w, fmt.Sprintf("unable to decode data movement request body: %s", err.Error()), http.StatusBadRequest)
		return
	}
	if err := dmreq.Validator(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user.Log.Info("  TrialRequest", "dmreq", dmreq)

	var dm *nnfv1alpha9.NnfDataMovement
	var dmKey string
	var err error
	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	if user.Mock {
		dmKey, dm, err = drvrReq.CreateMock(context.TODO(), dmreq)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s\n", err.Error()), http.StatusInternalServerError)
			return
		}

		err = drvrReq.DriveMock(context.TODO(), dmreq, dm)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s\n", err.Error()), http.StatusInternalServerError)
			return
		}
	} else {
		dmKey, dm, err = drvrReq.Create(context.TODO(), dmreq)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s\n", err.Error()), http.StatusInternalServerError)
			return
		}

		err = drvrReq.Drive(context.TODO(), dmreq, dm)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s\n", err.Error()), http.StatusInternalServerError)
			return
		}
	}

	// This is the v1.0 apiVersion output. See COPY_OFFLOAD_API_VERSION.
	fmt.Fprintf(w, "name=%s\n", dmKey)
}

func (user *UserHttp) ShutdownRequest(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "POST" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = user.validateMessage(w, req); apiVersion == "" {
		return
	}
	user.Log.Info("In ShutdownRequest", "version", apiVersion, "url", req.URL)

	items, err := user.GetActiveRequests()
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to list requests: %s\n", err.Error()), http.StatusInternalServerError)
		return
	}

	// If there are active requests, we cannot shutdown the server. Return StatusConflict (409)
	if len(items) > 0 {
		http.Error(w, "unable to shutdown server: requests in progress", http.StatusConflict)
		return
	}
	user.Log.Info("  ShutdownRequest")

	go func() {
		time.Sleep(1 * time.Second)
		if err := user.Server.Shutdown(context.Background()); err != nil {
			user.Log.Error(err, "server shutdown failed")
		}
	}()

	user.Log.Info("Server shutting down")
	fmt.Fprintf(w, "Server shutting down\n")
}
