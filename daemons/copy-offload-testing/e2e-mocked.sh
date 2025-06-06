#!/bin/bash

# Copyright 2024-2025 Hewlett Packard Enterprise Development LP
# Other additional copyright holders may be indicated within.
#
# The entirety of this work is licensed under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
#
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o pipefail

if [[ -z $SKIP_BUILD ]]; then
    make build-copy-offload-local || exit 1
    make -C ./daemons/lib-copy-offload tester || exit 1
fi
CO="./daemons/lib-copy-offload/tester ${SKIP_TLS:+-s}"
SRVR_NAME="localhost"
SRVR_PORT="4000"
SRVR="$SRVR_NAME:$SRVR_PORT"
PROTO="http"

CERTDIR=daemons/copy-offload-testing/certs
./tools/mk-usercontainer-secrets.sh -A -S $CERTDIR || exit 1

TOKEN_KEY=$CERTDIR/token_key.pem
JWT=$CERTDIR/token
go run ./daemons/copy-offload-testing/make-jwt/make-jwt.go -tokenkey "$TOKEN_KEY" -token "$JWT"

SRVR_CMD="./bin/nnf-copy-offload -addr $SRVR -mock ${SKIP_TOKEN:+-skip-token} ${SKIP_TLS:+-skip-tls}"
CURL_APIVER_HDR="Accepts-version: 1.0"
CA_KEY="$CERTDIR/ca/ca_key.pem"
if [[ -z $SKIP_TLS ]]; then
    PROTO="https"
    cacert="$CERTDIR/server/server_cert.pem"

    SRVR_WANTS_KEY=1
    SRVR_CMD_TLS_ARGS="-cert $cacert"
    CO_TLS_ARGS="-x $cacert"
    CURL_TLS_ARGS="--cacert $cacert"
fi
if [[ -z $SKIP_TOKEN ]]; then
    DW_WORKFLOW_TOKEN=$(<"$JWT")
    export DW_WORKFLOW_TOKEN
    CURL_BEARER_TOKEN_HDR="Authorization: Bearer $DW_WORKFLOW_TOKEN"

    token_key_file="$TOKEN_KEY"
    SRVR_CMD_TOKEN_ARGS="-tokenkey $token_key_file"
fi
if [[ -n $SRVR_WANTS_KEY ]]; then
    SRVR_CMD_TLS_ARGS="$SRVR_CMD_TLS_ARGS -cakey $CA_KEY"
fi

set -x
# shellcheck disable=SC2086
NNF_NODE_NAME=rabbit01 ENVIRONMENT=test DW_WORKFLOW_NAME=yellow DW_WORKFLOW_NAMESPACE=default $SRVR_CMD $SRVR_CMD_TOKEN_ARGS $SRVR_CMD_TLS_ARGS &
srvr_pid=$!
set +x
echo "Server pid is $srvr_pid, my pid is $$"

trap cleanup SIGINT SIGQUIT SIGABRT SIGTERM
# shellcheck disable=SC2317
cleanup() {
    echo "FAIL: trap"
    if [[ -n $srvr_pid ]]; then
        kill "$srvr_pid"
    fi
    if [[ -d $CERTDIR ]]; then
        rm -rf "$CERTDIR"
    fi
    exit 1
}

echo "Waiting for daemon to start"
cnt=10
while (( cnt > 0 )) ; do
    sleep 1
    # shellcheck disable=SC2086
    if output=$(curl -H "$CURL_APIVER_HDR" -H "$CURL_BEARER_TOKEN_HDR" $CURL_TLS_ARGS "$PROTO://$SRVR/hello"); then
        break
    fi
    (( cnt = cnt - 1 ))
done
if [[ $output != "hello back at ya" ]]; then
    echo "FAIL: curl did not get expected 'hello' response"
    kill "$srvr_pid"
    exit 1
fi

export NNF_CONTAINER_PORTS="$SRVR_PORT"
export NNF_CONTAINER_LAUNCHER="$SRVR_NAME"

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -H); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "hello back at ya" ]]; then
    echo "FAIL: Tester did not get expected 'hello' response"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -l); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from list before any jobs have been submitted"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if output=$($CO $CO_TLS_ARGS -c nnf-copy-offload-node-2); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "unable to cancel request: request not found" ]]; then
    echo "FAIL: Expected cancel to not find my request before any jobs have been submitted"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! job1=$($CO $CO_TLS_ARGS -o -S /mnt/nnf/ooo -D /lus/foo); then
    echo "line $LINENO output: $job1"
    cleanup
fi
if [[ $job1 != "mock-rabbit-01--nnf-copy-offload-node-0" ]]; then
    echo "FAIL: Unexpected output from copy. Got ($job1)."
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -q -j $job1); then
    echo "line $LINENO output: $output"
    cleanup
fi
echo "GETREQUEST:"
echo "$output"

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -l); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $(echo "$output" | wc -l) -ne 1 ]]; then
    echo "FAIL: Unexpected output from list. Got ($output)."
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -c "$job1"); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from cancel $job1"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -l); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from list after all jobs have been removed"
    kill "$srvr_pid"
    exit 1
fi

if [[ -z $SKIP_TLS ]]; then
    SAY_CURL="Verify that TLS args are required for curl. Expect curl to fail here."
    SAY_CURL_ERR="FAIL: Expected curl to get failure when not specifying the TLS cert/key"
    SAY_TESTER_ERR="FAIL: Expected test tool to get failure when not specifying the TLS cert/key"
    SAY_TESTER="Verify that TLS args are required for test tool. Expect it to fail here."

    echo
    echo "$SAY_CURL"
    echo
    # shellcheck disable=SC2086
    if output=$(curl -H "$CURL_APIVER_HDR" $USE_CURL_ARGS "$PROTO://$SRVR/hello" 2>&1); then
        echo "$SAY_CURL_ERR"
        kill "$srvr_pid"
        exit 1
    fi
    echo "output($output)"
    echo
    echo "$SAY_TESTER"
    echo
    # shellcheck disable=SC2086
    if output=$($CO $USE_TESTER_ARGS -l 2>&1); then
        echo "$SAY_TESTER_ERR"
        kill "$srvr_pid"
        exit 1
    fi
    echo "output($output)"
fi

echo
echo "PASS: Success"
echo "Requesting server shutdown"

# Send shutdown request
# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS -X); then
    echo "line $LINENO output: $output"
    cleanup
fi
echo "Shutdown response: $output"

# Wait for the server to exit
wait "$srvr_pid"
server_exit_code=$?

if [[ $server_exit_code -ne 0 ]]; then
    echo "FAIL: Server did not exit cleanly (exit code $server_exit_code)"
    exit 1
fi

if [[ -d $CERTDIR ]]; then
    rm -rf "$CERTDIR"
fi
exit 0

