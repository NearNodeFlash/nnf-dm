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

make build-copy-offload-local || exit 1

make -C ./daemons/lib-copy-offload tester || exit 1
CO="./daemons/lib-copy-offload/tester ${SKIP_TLS:+-s}"
SRVR="localhost:4000"
PROTO="http"

CERTDIR=daemons/copy-offload-testing/certs
./daemons/copy-offload-testing/gen_certs.sh $CERTDIR || exit 1

SRVR_CMD="./bin/nnf-copy-offload -addr $SRVR -mock ${SKIP_TOKEN:+-skip-token} ${SKIP_TLS:+-skip-tls}"
CURL_APIVER_HDR="Accepts-version: 1.0"
CA_KEY="$CERTDIR/ca/private/ca_key.pem"
if [[ -z $SKIP_TLS ]]; then
    PROTO="https"
    cacert="$CERTDIR/server/server_cert.pem"
    clientcert="$CERTDIR/client/client_cert.pem"

    SRVR_WANTS_KEY=1
    SRVR_CMD_TLS_ARGS="-cert $cacert"
    CO_TLS_ARGS="-x $cacert"
    CURL_TLS_ARGS="--cacert $cacert"

    # Enable mTLS?
    if [[ -z $SKIP_MTLS ]]; then
        SRVR_CMD_MTLS_ARGS="-clientcert $clientcert"
        CO_MTLS_ARGS="-z $clientcert -y $CA_KEY"
        CURL_MTLS_ARGS="--cert $clientcert --key $CA_KEY"
    fi
fi
if [[ -z $SKIP_TOKEN ]]; then
    SRVR_WANTS_KEY=1
    TOKEN=$(<"$CERTDIR/client/token")
    CURL_BEARER_TOKEN_HDR="Authorization: Bearer $TOKEN"
    CO_TLS_ARGS="$CO_TLS_ARGS -t $CERTDIR/client/token"
fi
if [[ -n $SRVR_WANTS_KEY ]]; then
    SRVR_CMD_TLS_ARGS="$SRVR_CMD_TLS_ARGS -cakey $CA_KEY"
fi

set -x
# shellcheck disable=SC2086
NNF_NODE_NAME=rabbit01 $SRVR_CMD $SRVR_CMD_TLS_ARGS $SRVR_CMD_MTLS_ARGS &
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
set -x
while (( cnt > 0 )) ; do
    sleep 1
    # shellcheck disable=SC2086
    if output=$(curl -H "$CURL_APIVER_HDR" -H "$CURL_BEARER_TOKEN_HDR" $CURL_TLS_ARGS $CURL_MTLS_ARGS "$PROTO://$SRVR/hello"); then
        break
    fi
    (( cnt = cnt - 1 ))
done
if [[ $output != "hello back at ya" ]]; then
    echo "FAIL: curl did not get expected 'hello' response"
    kill "$srvr_pid"
    exit 1
fi
set +x

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS $CO_MTLS_ARGS -l "$SRVR"); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from list before any jobs have been submitted"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS $CO_MTLS_ARGS -c nnf-copy-offload-node-2 "$SRVR"); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from cancel before any jobs have been submitted"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! job1=$($CO $CO_TLS_ARGS $CO_MTLS_ARGS -o -C compute-01 -W yellow -S /mnt/nnf/ooo -D /lus/foo "$SRVR"); then
    echo "line $LINENO output: $job1"
    cleanup
fi
if [[ $job1 != "nnf-copy-offload-node-0" ]]; then
    echo "FAIL: Unexpected output from copy. Got ($job1)."
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS $CO_MTLS_ARGS -l "$SRVR"); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $(echo "$output" | wc -l) -ne 1 ]]; then
    echo "FAIL: Unexpected output from list. Got ($output)."
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS $CO_MTLS_ARGS -c "$job1" "$SRVR"); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from cancel $job1"
    kill "$srvr_pid"
    exit 1
fi

# shellcheck disable=SC2086
if ! output=$($CO $CO_TLS_ARGS $CO_MTLS_ARGS -l "$SRVR"); then
    echo "line $LINENO output: $output"
    cleanup
fi
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from list after all jobs have been removed"
    kill "$srvr_pid"
    exit 1
fi

if [[ -z $SKIP_TLS ]]; then
    if [[ -z $SKIP_MTLS ]]; then
        SAY_CURL="Verify that mTLS args are required for curl. Expect curl to fail here."
        USE_CURL_ARGS="$CURL_TLS_ARGS"
        SAY_CURL_ERR="FAIL: Expected curl to get failure when not specifying the mTLS cert"

        SAY_TESTER="Verify that mTLS args are required for test tool. Expect it to fail here."
        USE_TESTER_ARGS="$CO_TLS_ARGS"
        SAY_TESTER_ERR="FAIL: Expected test tool to get failure when not specifying the mTLS cert"
   else
        SAY_CURL="Verify that TLS args are required for curl. Expect curl to fail here."
        SAY_CURL_ERR="FAIL: Expected curl to get failure when not specifying the TLS cert/key"

        SAY_TESTER_ERR="FAIL: Expected test tool to get failure when not specifying the TLS cert/key"
        SAY_TESTER="Verify that TLS args are required for test tool. Expect it to fail here."
    fi

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
    if output=$($CO $USE_TESTER_ARGS -l "$SRVR" 2>&1); then
        echo "$SAY_TESTER_ERR"
        kill "$srvr_pid"
        exit 1
    fi
    echo "output($output)"
fi

echo
echo "PASS: Success"
echo "Kill server $srvr_pid"
kill "$srvr_pid"
if [[ -d $CERTDIR ]]; then
    rm -rf "$CERTDIR"
fi
exit 0

