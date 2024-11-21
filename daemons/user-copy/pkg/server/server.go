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
	"os"
	"path/filepath"

	"github.com/NearNodeFlash/nnf-dm/daemons/user-copy/pkg/driver"
	"github.com/go-logr/logr"
)

type UserHttp struct {
	Log       logr.Logger
	Drvr      *driver.Driver
	SetupOnly bool
	InTest    bool
}

func (user *UserHttp) Hello(w http.ResponseWriter, req *http.Request) {
	user.Log.Info("Hello")
	fmt.Fprintf(w, "hello back at ya\n")
}

func (user *UserHttp) ListRequests(w http.ResponseWriter, req *http.Request) {

	if req.Method != "GET" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}

	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	_, err := drvrReq.ListRequests(context.TODO())
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to list requests: %s\n", err.Error()), http.StatusInternalServerError)
		return
	}
	http.Error(w, "", http.StatusNoContent)
}

func (user *UserHttp) CancelRequest(w http.ResponseWriter, req *http.Request) {

	if req.Method != "DELETE" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}
	user.Log.Info("In DELETE", "url", req.URL)
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

	if req.Method != "POST" {
		http.Error(w, "method not supported", http.StatusNotImplemented)
		return
	}

	var dmreq driver.DMRequest
	if err := json.NewDecoder(req.Body).Decode(&dmreq); err != nil {
		http.Error(w, fmt.Sprintf("unable to decode data movement request body: %s", err.Error()), http.StatusBadRequest)
		return
	}
	if err := dmreq.Validator(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, inTest := os.LookupEnv("USING_HTTPTEST"); inTest {
		http.Error(w, "", http.StatusNoContent)
		return
	}

	drvrReq := driver.DriverRequest{Drvr: user.Drvr}
	dm, err := drvrReq.Create(context.TODO(), dmreq)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s\n", err.Error()), http.StatusInternalServerError)
		return
	}

	if user.SetupOnly {
		user.Log.Info("Stopping after setup")
		user.Log.Info("NnfDataMovement", "name", dm.GetName())
		user.Log.Info("NnfDataMovement.Spec", "value", fmt.Sprintf("%+v", dm.Spec))
		user.Log.Info("NnfDataMovement.Spec.UserConfig", "value", fmt.Sprintf("%+v", dm.Spec.UserConfig))
		user.Log.Info("NnfDataMovement.Spec.Source", "value", fmt.Sprintf("%+v", dm.Spec.Source))
		user.Log.Info("NnfDataMovement.Spec.Destination", "value", fmt.Sprintf("%+v", dm.Spec.Destination))
		fmt.Fprintf(w, "{\"name\": \"%s\"}\n", dm.GetName())
		return
	}

	err = drvrReq.Drive(context.TODO(), dmreq, dm)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s\n", err.Error()), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "{\"name\": \"%s\"}\n", dm.GetName())
}
