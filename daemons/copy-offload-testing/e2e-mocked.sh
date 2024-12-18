#!/bin/bash

# Copyright 2024 Hewlett Packard Enterprise Development LP
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

set -e
set -o pipefail

make build-copy-offload-local

make -C ./daemons/lib-copy-offload tester
CO="./daemons/lib-copy-offload/tester ${SKIP_TLS:+-s}"
SRVR="localhost:4000"
PROTO="http"

SRVR_CMD="./bin/nnf-copy-offload -addr $SRVR -mock ${SKIP_TLS:+-skip-tls}"
unset CERTDIR
unset CURLCERTS
if [[ -z $SKIP_TLS ]]; then
    PROTO="https"
    CERTDIR=daemons/copy-offload-testing/certs
    ./daemons/copy-offload-testing/gen_certs.sh $CERTDIR
    cacert="$CERTDIR/server/server_cert.pem"
    cakey="$CERTDIR/ca/private/ca_key.pem"
    clientcert="$CERTDIR/client/client_cert.pem"

    SRVR_CMD="$SRVR_CMD -cert $cacert -cakey $cakey"
    CO="$CO -x $cacert -y $cakey"
    CURLCERTS="--cacert $cacert --key $cakey"

    # Enable mTLS?
    if [[ -z $SKIP_MTLS ]]; then
        SRVR_CMD="$SRVR_CMD -clientcert $clientcert"
        CO="$CO -z $clientcert"
        CURLCERTS="$CURLCERTS --cert $clientcert"
    fi
fi

NNF_NODE_NAME=rabbit01 $SRVR_CMD &
srvr_pid=$!
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
while : ; do
    sleep 1
    # shellcheck disable=SC2086
    if curl -H "Accepts-version: 1.0" $CURLCERTS "$PROTO://$SRVR/hello"; then
        break
    fi
done

output=$($CO -l "$SRVR")
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from list before any jobs have been submitted"
    kill "$srvr_pid"
    exit 1
fi

output=$($CO -c nnf-copy-offload-node-2 "$SRVR")
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from cancel before any jobs have been submitted"
    kill "$srvr_pid"
    exit 1
fi

job1=$($CO -o -C compute-01 -W yellow -S /mnt/nnf/ooo -D /lus/foo "$SRVR")
if [[ $job1 != "nnf-copy-offload-node-0" ]]; then
    echo "FAIL: Unexpected output from copy. Got ($job1)."
    kill "$srvr_pid"
    exit 1
fi

output=$($CO -l "$SRVR")
if [[ $(echo "$output" | wc -l) -ne 1 ]]; then
    echo "FAIL: Unexpected output from list. Got ($output)."
    kill "$srvr_pid"
    exit 1
fi

output=$($CO -c "$job1" "$SRVR")
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from cancel $job1"
    kill "$srvr_pid"
    exit 1
fi

output=$($CO -l "$SRVR")
if [[ $output != "" ]]; then
    echo "FAIL: Expected empty output from list after all jobs have been removed"
    kill "$srvr_pid"
    exit 1
fi

echo "Kill server $srvr_pid"
kill "$srvr_pid"
if [[ -d $CERTDIR ]]; then
    rm -rf "$CERTDIR"
fi
echo "PASS: Success"
exit 0

