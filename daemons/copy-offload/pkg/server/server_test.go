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
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	zapcr "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/NearNodeFlash/nnf-dm/daemons/copy-offload/pkg/driver"
)

// A bearer token and the DER form of the key that was used to sign it.
var bearerToken1 = ""
var derKey1 = []byte("")

// A different, but otherwise valid, bearer token and its DER key.
var bearerToken2 = ""
var derKey2 = []byte("")

// A different bearer token and DER key using a different signing algorithm.
var bearerTokenAlg2 = ""
var derKeyAlg2 = []byte("")

// Fill in the tokens/keys prior to running the tests.
func TestMain(m *testing.M) {
	err := createTokensAndKeys()
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func createTokensAndKeys() error {
	var err error

	createTokenAndKey := func(signingMethod *jwt.SigningMethodHMAC, verifiesOK bool) (string, []byte, error) {
		createToken := func(key []byte, method jwt.SigningMethod) (string, error) {
			token := jwt.NewWithClaims(method,
				jwt.MapClaims{
					"sub": "user-container",
					"iat": time.Now().Unix(),
				})

			tokenString, err := token.SignedString(key)
			if err != nil {
				return "", fmt.Errorf("Failure from SignedString: %w", err)
			}
			return tokenString, nil
		}

		createKey := func() ([]byte, error) {
			privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
			if err != nil {
				return []byte{}, fmt.Errorf("Failure from GenerateKey: %w", err)
			}
			privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
			if err != nil {
				return []byte{}, fmt.Errorf("Failure from MarshalPKCS8PrivateKey: %w", err)
			}
			return privBytes, nil
		}

		privKey, err := createKey()
		if err != nil {
			return "", []byte(""), err
		}
		tokenString, err := createToken(privKey, signingMethod)
		if err != nil {
			return "", []byte(""), err
		}
		if verifiesOK {
			httpHandler := &UserHttp{KeyBytes: privKey}
			if err := httpHandler.verifyToken(tokenString); err != nil {
				return "", []byte(""), fmt.Errorf("Failure in real verifyToken: %w", err)
			}
		}
		return tokenString, privKey, nil
	}

	// First valid key/token.
	bearerToken1, derKey1, err = createTokenAndKey(jwt.SigningMethodHS256, true)
	if err != nil {
		return fmt.Errorf("Failure creating token/key #1: %w", err)
	}
	// Second valid key/token, signed the same way.
	bearerToken2, derKey2, err = createTokenAndKey(jwt.SigningMethodHS256, true)
	if err != nil {
		return fmt.Errorf("Failure creating token/key #2: %w", err)
	}
	// Use a different signing method for this key/token. Though still valid,
	// the token is not signed the way the server expects.
	bearerTokenAlg2, derKeyAlg2, err = createTokenAndKey(jwt.SigningMethodHS512, false)
	if err != nil {
		return fmt.Errorf("Failure creating token/key #3: %w", err)
	}
	return nil
}

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
			wantText:   "unable to cancel request: request not found\n",
			wantStatus: http.StatusNotFound,
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
	drvr := &driver.Driver{Log: crLog, Mock: true}
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
	drvr := &driver.Driver{Log: crLog, Mock: true}
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

	stringWanted = ""
	t.Run("list remaining jobs", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for {
			time.Sleep(1 * time.Second)
			select {
			case <-ctx.Done():
				t.Errorf("timeout waiting for jobs to finish")
				return
			default:
				request, _ := http.NewRequest(http.MethodGet, "/list", nil)
				request.Header.Set("Accepts-version", "1.0")
				response := httptest.NewRecorder()

				httpHandler.ListRequests(response, request)

				res := response.Result()
				got := response.Body.String()

				chopGot := strings.TrimRight(got, "\n")
				if res.StatusCode != http.StatusOK {
					fmt.Printf("got status %d, want status %d\n", res.StatusCode, http.StatusOK)
					continue
				}
				if chopGot != stringWanted {
					fmt.Printf("got %q, want %q\n", chopGot, stringWanted)
					continue
				}
				return
			}
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

func TestG_BearerToken(t *testing.T) {

	t.Run("accepts valid bearer token when using matching key", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		request.Header.Set("Authorization", "Bearer "+bearerToken1)
		response := httptest.NewRecorder()

		httpHandler := &UserHttp{Log: setupLog(), KeyBytes: derKey1}

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

func TestH_BearerTokenNegatives(t *testing.T) {

	t.Run("fails when bearer token is expected but is not correct", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		request.Header.Set("Authorization", "Bearer "+bearerToken2)
		response := httptest.NewRecorder()

		httpHandler := &UserHttp{Log: setupLog(), KeyBytes: derKey1}

		httpHandler.Hello(response, request)

		res := response.Result()
		got := response.Body.String()
		statusWant := http.StatusUnauthorized

		if res.StatusCode != statusWant {
			t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
		}
		if !strings.Contains(got, "signature is invalid") {
			t.Errorf("got %s, wanted message about invalid signature", got)
		}
	})

	t.Run("fails when key is invalid", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		request.Header.Set("Authorization", "Bearer "+bearerToken1)
		response := httptest.NewRecorder()

		invalidKey := derKey1
		invalidKey = append(invalidKey, byte('='))
		httpHandler := &UserHttp{Log: setupLog(), KeyBytes: invalidKey}

		httpHandler.Hello(response, request)

		res := response.Result()
		got := response.Body.String()
		statusWant := http.StatusUnauthorized

		if res.StatusCode != statusWant {
			t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
		}
		if !strings.Contains(got, "signature is invalid") {
			t.Errorf("got %s, wanted message about key being of invalid type", got)
		}
	})

	t.Run("fails when key doesn't match", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		request.Header.Set("Authorization", "Bearer "+bearerToken1)
		response := httptest.NewRecorder()

		httpHandler := &UserHttp{Log: setupLog(), KeyBytes: derKey2}

		httpHandler.Hello(response, request)

		res := response.Result()
		got := response.Body.String()
		statusWant := http.StatusUnauthorized

		if res.StatusCode != statusWant {
			t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
		}
		if !strings.Contains(got, "signature is invalid") {
			t.Errorf("got %s, wanted message about key being of invalid type", got)
		}
	})

	t.Run("fails when bearer token is expected but is not specified", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		response := httptest.NewRecorder()

		httpHandler := &UserHttp{Log: setupLog(), KeyBytes: derKey1}

		httpHandler.Hello(response, request)

		res := response.Result()
		got := response.Body.String()
		statusWant := http.StatusUnauthorized

		if res.StatusCode != statusWant {
			t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
		}
		if !strings.Contains(got, "unauthorized") {
			t.Errorf("got %s, wanted unauthorized", got)
		}
	})

	t.Run("fails when bearer token is signed with a different algorithm", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		request.Header.Set("Accepts-version", "1.0")
		request.Header.Set("Authorization", "Bearer "+string(bearerTokenAlg2))
		response := httptest.NewRecorder()

		httpHandler := &UserHttp{Log: setupLog(), KeyBytes: derKeyAlg2}

		httpHandler.Hello(response, request)

		res := response.Result()
		got := response.Body.String()
		statusWant := http.StatusUnauthorized

		if res.StatusCode != statusWant {
			t.Errorf("got status %d, want status %d", res.StatusCode, statusWant)
		}
		if !strings.Contains(got, "unexpected signing method") {
			t.Errorf("got %s, wanted a message about an unexpected signing method", got)
		}
	})
}

// Just touch ginkgo, so it's here to interpret any ginkgo args from
// "make test", so that doesn't fail on this test file.
var _ = BeforeSuite(func() {})
