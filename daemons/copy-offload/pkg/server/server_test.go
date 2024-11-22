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

func TestHello(t *testing.T) {
	t.Run("returns hello response", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
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

func TestListRequests(t *testing.T) {
	testCases := []struct {
		name       string
		method     string
		wantText   string
		wantStatus int
	}{
		{
			name:       "returns status-no-content",
			method:     http.MethodGet,
			wantText:   "\n",
			wantStatus: http.StatusNoContent,
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

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(test.method, "/list", nil)
			response := httptest.NewRecorder()

			crLog := setupLog()
			drvr := &driver.Driver{Log: crLog}
			httpHandler := &UserHttp{Log: crLog, Drvr: drvr}

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

func TestCancelRequest(t *testing.T) {
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

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(test.method, "/cancel/nnf-copy-offload-node-9ae2a136-4", nil)
			response := httptest.NewRecorder()

			crLog := setupLog()
			drvr := &driver.Driver{Log: crLog}
			httpHandler := &UserHttp{Log: crLog, Drvr: drvr}

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

func TestTrialRequest(t *testing.T) {
	testCases := []struct {
		name       string
		method     string
		body       []byte
		wantText   string
		wantStatus int
	}{
		{
			name:       "returns status-no-content",
			method:     http.MethodPost,
			body:       []byte("{\"computeName\": \"rabbit-compute-3\", \"workflowName\": \"yellow\", \"sourcePath\": \"/mnt/nnf/dc51a384-99bd-4ef1-8444-4ee3b0cdc8a8-0\", \"destinationPath\": \"/lus/global/dean/foo\", \"dryrun\": true}"),
			wantText:   "\n",
			wantStatus: http.StatusNoContent,
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

	t.Setenv("USING_HTTPTEST", "true")

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var readerBody io.Reader = nil
			if len(test.body) > 0 {
				readerBody = bytes.NewReader(test.body)
			}
			request, _ := http.NewRequest(test.method, "/trial", readerBody)

			response := httptest.NewRecorder()

			crLog := setupLog()
			httpHandler := &UserHttp{Log: crLog}

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

// Just touch ginkgo, so it's here to interpret any ginkgo args from
// "make test", so that doesn't fail on this test file.
var _ = BeforeSuite(func() {})
