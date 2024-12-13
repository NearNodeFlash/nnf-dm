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
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	zapcr "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/driver"
)

func setupLog() logr.Logger {
	encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	zaplogger := zapcr.New(zapcr.Encoder(encoder), zapcr.UseDevMode(true))
	ctrl.SetLogger(zaplogger)

	// controllerruntime logger.
	crLog := ctrl.Log.WithName("copy-offload-test")
	return crLog
}

func TestA_Hello(t *testing.T) {
	t.Run("returns hello response", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		response := httptest.NewRecorder()

		httpHandler := &UserHttp{Log: setupLog()}

		httpHandler.Hello(response, request)

		res := response.Result()
		got := response.Body.String()
		want := "hello back at ya\n"
		statusWant := http.StatusOK

		if res.StatusCode != statusWant {
			t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestB_ListRequests(t *testing.T) {
	testCases := []struct {
		name       string
		method     string
		wantText   string
		wantStatus int
	}{
		{
			name:       "returns status-no-content",
			method:     http.MethodGet,
			wantText:   "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns status-not-implemented for POST",
			method:     http.MethodPost,
			wantText:   "method not supported\n",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "returns status-not-implemented for PUT",
			method:     http.MethodPut,
			wantText:   "method not supported\n",
			wantStatus: http.StatusNotImplemented,
		},
	}

	crLog := setupLog()
	drvr := &driver.Driver{Log: crLog, Mock: true}
	httpHandler := &UserHttp{Log: crLog, Drvr: drvr, Mock: true}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(test.method, "/list", nil)
			request.Header.Set("Accepts-version", "1.0")
			response := httptest.NewRecorder()

			httpHandler.ListRequests(response, request)

			res := response.Result()
			got := response.Body.String()

			if res.StatusCode != test.wantStatus {
				t.Errorf("got status %d, want status %d", res.StatusCode, test.wantStatus)
			}
			if got != test.wantText {
				t.Errorf("got %q, want %q", got, test.wantText)
			}
		})
	}
}

func TestC_CancelRequest(t *testing.T) {
	testCases := []struct {
		name       string
		method     string
		wantText   string
		wantStatus int
	}{
		{
			name:       "returns status-no-content",
			method:     http.MethodDelete,
			wantText:   "\n",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "returns status-not-implemented for GET",
			method:     http.MethodGet,
			wantText:   "method not supported\n",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "returns status-not-implemented for PUT",
			method:     http.MethodPut,
			wantText:   "method not supported\n",
			wantStatus: http.StatusNotImplemented,
		},
	}

	crLog := setupLog()
	drvr := &driver.Driver{Log: crLog, Mock: true}
	httpHandler := &UserHttp{Log: crLog, Drvr: drvr, Mock: true}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(test.method, "/cancel/nnf-copy-offload-node-9ae2a136-4", nil)
			request.Header.Set("Accepts-version", "1.0")
			response := httptest.NewRecorder()

			httpHandler.CancelRequest(response, request)

			res := response.Result()
			got := response.Body.String()

			if res.StatusCode != test.wantStatus {
				t.Errorf("got status %d, want status %d", res.StatusCode, test.wantStatus)
			}
			if got != test.wantText {
				t.Errorf("got %q, want %q", got, test.wantText)
			}
		})
	}
}

func TestD_TrialRequest(t *testing.T) {
	testCases := []struct {
		name       string
		method     string
		body       []byte
		wantText   string
		wantStatus int
	}{
		{
			name:       "returns status-ok",
			method:     http.MethodPost,
			body:       []byte("{\"computeName\": \"rabbit-compute-3\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			wantText:   "name=nnf-copy-offload-node-0\n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns status-bad-request",
			method:     http.MethodPost,
			body:       []byte("{\"unknown\": 1}"),
			wantText:   "compute name must be supplied\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns status-not-implemented for GET",
			method:     http.MethodGet,
			wantText:   "method not supported\n",
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "returns status-not-implemented for PUT",
			method:     http.MethodPut,
			wantText:   "method not supported\n",
			wantStatus: http.StatusNotImplemented,
		},
	}

	crLog := setupLog()
	drvr := &driver.Driver{Log: crLog, RabbitName: "rabbit-1", Mock: true}
	httpHandler := &UserHttp{Log: crLog, Drvr: drvr, Mock: true}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var readerBody io.Reader = nil
			if len(test.body) > 0 {
				readerBody = bytes.NewReader(test.body)
			}
			request, _ := http.NewRequest(test.method, "/trial", readerBody)
			request.Header.Set("Accepts-version", "1.0")
			response := httptest.NewRecorder()

			httpHandler.TrialRequest(response, request)

			res := response.Result()
			got := response.Body.String()

			if res.StatusCode != test.wantStatus {
				t.Errorf("got status %d, want status %d", res.StatusCode, test.wantStatus)
			}
			if got != test.wantText {
				t.Errorf("got %q, want %q", got, test.wantText)
			}
		})
	}
}

func TestE_Lifecycle(t *testing.T) {

	scheduleJobs := []struct {
		name       string
		method     string
		body       []byte
		wantText   string
		wantStatus int
	}{
		{
			name:       "schedule job 1",
			method:     http.MethodPost,
			body:       []byte("{\"computeName\": \"rabbit-compute-3\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			wantText:   "name=nnf-copy-offload-node-0\n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "schedule job 2",
			method:     http.MethodPost,
			body:       []byte("{\"computeName\": \"rabbit-compute-4\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			wantText:   "name=nnf-copy-offload-node-1\n",
			wantStatus: http.StatusOK,
		},
		{
			name:       "schedule job 3",
			method:     http.MethodPost,
			body:       []byte("{\"computeName\": \"rabbit-compute-5\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			wantText:   "name=nnf-copy-offload-node-2\n",
			wantStatus: http.StatusOK,
		},
	}

	crLog := setupLog()
	drvr := &driver.Driver{Log: crLog, RabbitName: "rabbit-1", Mock: true}
	httpHandler := &UserHttp{Log: crLog, Drvr: drvr, Mock: true}

	var listWanted []string
	var jobCount int = 0
	for _, test := range scheduleJobs {
		t.Run(test.name, func(t *testing.T) {
			var readerBody io.Reader = nil
			if len(test.body) > 0 {
				readerBody = bytes.NewReader(test.body)
			}
			request, _ := http.NewRequest(test.method, "/trial", readerBody)
			request.Header.Set("Accepts-version", "1.0")
			response := httptest.NewRecorder()

			httpHandler.TrialRequest(response, request)

			res := response.Result()
			got := response.Body.String()

			if res.StatusCode != test.wantStatus {
				t.Errorf("got status %d, want status %d", res.StatusCode, test.wantStatus)
			}
			if got != test.wantText {
				t.Errorf("got %q, want %q", got, test.wantText)
			} else {
				parts := strings.Split(got, "=")
				listWanted = append(listWanted, strings.TrimRight(parts[1], "\n"))
				jobCount += 1
			}
		})
	}

	stringWanted := strings.Join(listWanted, ",")
	t.Run("list all jobs", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/list", nil)
		request.Header.Set("Accepts-version", "1.0")
		response := httptest.NewRecorder()

		httpHandler.ListRequests(response, request)

		res := response.Result()
		got := response.Body.String()
		chopGot := strings.TrimRight(got, "\n")

		if res.StatusCode != http.StatusOK {
			t.Errorf("got status %d, want status %d", res.StatusCode, http.StatusOK)
		}
		if chopGot != stringWanted {
			t.Errorf("got %q, want %q", chopGot, stringWanted)
		}
	})

	t.Run("cancel job", func(t *testing.T) {
		// Go bug? If I try to dynamically build the url for this request I will
		// get a null pointer reference in CancelRequest(), where 'req' will
		// be null.
		request, _ := http.NewRequest(http.MethodDelete, "/cancel/nnf-copy-offload-node-0", nil)
		request.Header.Set("Accepts-version", "1.0")
		response := httptest.NewRecorder()

		httpHandler.CancelRequest(response, request)

		res := response.Result()
		got := response.Body.String()

		if res.StatusCode != http.StatusNoContent {
			t.Errorf("got status %d, want status %d", res.StatusCode, http.StatusNoContent)
		}
		if got != "\n" {
			t.Errorf("got %q, want %q", got, "(newline)")
		}
	})

	stringWanted = strings.Join(listWanted[1:], ",")
	t.Run("list remaining jobs", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/list", nil)
		request.Header.Set("Accepts-version", "1.0")
		response := httptest.NewRecorder()

		httpHandler.ListRequests(response, request)

		res := response.Result()
		got := response.Body.String()
		chopGot := strings.TrimRight(got, "\n")

		if res.StatusCode != http.StatusOK {
			t.Errorf("got status %d, want status %d", res.StatusCode, http.StatusOK)
		}
		if chopGot != stringWanted {
			t.Errorf("got %q, want %q", chopGot, stringWanted)
		}
	})
}

func TestF_BadAPIVersion(t *testing.T) {

	crLog := setupLog()
	drvr := &driver.Driver{Log: crLog, Mock: true}
	httpHandler := &UserHttp{Log: crLog, Drvr: drvr, Mock: true}

	testCases := []struct {
		name           string
		method         string
		url            string
		handler        func(http.ResponseWriter, *http.Request)
		body           []byte
		skipApiVersion bool
	}{
		{
			name:    "bad api version for hello",
			method:  http.MethodGet,
			url:     "/hello",
			handler: httpHandler.Hello,
		},
		{
			name:           "skip api version for hello",
			method:         http.MethodGet,
			url:            "/hello",
			handler:        httpHandler.Hello,
			skipApiVersion: true,
		},
		{
			name:    "bad api version for list",
			method:  http.MethodGet,
			url:     "/list",
			handler: httpHandler.ListRequests,
		},
		{
			name:           "skip api version for list",
			method:         http.MethodGet,
			url:            "/list",
			handler:        httpHandler.ListRequests,
			skipApiVersion: true,
		},
		{
			name:    "bad api version for cancel",
			method:  http.MethodDelete,
			url:     "/cancel/nnf-copy-offload-node-9ae2a136-4",
			handler: httpHandler.CancelRequest,
		},
		{
			name:           "skip api version for cancel",
			method:         http.MethodDelete,
			url:            "/cancel/nnf-copy-offload-node-9ae2a136-4",
			handler:        httpHandler.CancelRequest,
			skipApiVersion: true,
		},
		{
			name:    "bad api version for copy",
			method:  http.MethodPost,
			url:     "/trial",
			body:    []byte("{\"computeName\": \"rabbit-compute-3\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			handler: httpHandler.TrialRequest,
		},
		{
			name:           "skip api version for copy",
			method:         http.MethodPost,
			url:            "/trial",
			body:           []byte("{\"computeName\": \"rabbit-compute-3\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			handler:        httpHandler.TrialRequest,
			skipApiVersion: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var readerBody io.Reader = nil
			if len(test.body) > 0 {
				readerBody = bytes.NewReader(test.body)
			}
			request, _ := http.NewRequest(test.method, test.url, readerBody)
			if !test.skipApiVersion {
				request.Header.Set("Accepts-version", "0.0")
			}
			response := httptest.NewRecorder()

			test.handler(response, request)

			res := response.Result()
			got := response.Body.String()
			statusWant := http.StatusNotAcceptable
			wantPrefix := "Valid versions: "

			if res.StatusCode != statusWant {
				t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
			}
			if !strings.HasPrefix(got, wantPrefix) {
				t.Errorf("got %q, want \"%s[...]\"", got, wantPrefix)
			}
		})
	}
}

// Just touch ginkgo, so it's here to interpret any ginkgo args from
// "make test", so that doesn't fail on this test file.
var _ = BeforeSuite(func() {})
