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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/driver"
	nnfv1alpha4 "github.com/NearNodeFlash/nnf-sos/api/v1alpha4"
	"github.com/go-logr/logr"
)

type UserHttp struct {
	Log    logr.Logger
	Drvr   *driver.Driver
	InTest bool
	Mock   bool
}

func validateVersion(w http.ResponseWriter, req *http.Request) string {
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
	if apiVersion = validateVersion(w, req); apiVersion == "" {
		return
	}

	user.Log.Info("Hello")
	fmt.Fprintf(w, "hello back at ya\n")
}

func (user *UserHttp) ListRequests(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "GET" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = validateVersion(w, req); apiVersion == "" {
		return
	}

	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	items, err := drvrReq.ListRequests(context.TODO())
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to list requests: %s\n", err.Error()), http.StatusInternalServerError)
		return
	}
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
	if apiVersion = validateVersion(w, req); apiVersion == "" {
		return
	}
	user.Log.Info("In DELETE", "version", apiVersion, "url", req.URL)
	urlParts, err := url.Parse(req.URL.String())
	if err != nil {
		http.Error(w, "unable to parse URL", http.StatusBadRequest)
		return
	}
	name := filepath.Base(urlParts.Path)

	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	if err := drvrReq.CancelRequest(context.TODO(), name); err != nil {
		http.Error(w, fmt.Sprintf("unable to cancel request: %s\n", err.Error()), http.StatusBadRequest)
		return
	}
	http.Error(w, "", http.StatusNoContent)
}

func (user *UserHttp) TrialRequest(w http.ResponseWriter, req *http.Request) {
	var apiVersion string
	if req.Method != "POST" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	if apiVersion = validateVersion(w, req); apiVersion == "" {
		return
	}
	user.Log.Info("In TrialRequest", "version", apiVersion, "url", req.URL)

	var dmreq driver.DMRequest
	if err := json.NewDecoder(req.Body).Decode(&dmreq); err != nil {
		http.Error(w, fmt.Sprintf("unable to decode data movement request body: %s", err.Error()), http.StatusBadRequest)
		return
	}
	user.Log.Info("  TrialRequest", "dmreq", dmreq)
	if err := dmreq.Validator(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var dm *nnfv1alpha4.NnfDataMovement
	var err error
	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	if user.Mock {
		dm, err = drvrReq.CreateMock(context.TODO(), dmreq)
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
		dm, err = drvrReq.Create(context.TODO(), dmreq)
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
	fmt.Fprintf(w, "name=%s\n", dm.GetName())
}
