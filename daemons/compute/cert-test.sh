#!/bin/bash

# Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
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


SERVICE_ACCOUNT=nnf-dm-controller-manager
NAMESPACE=nnf-dm-system

echo "Obtaining token and secret for service account ${SERVICE_ACCOUNT}..."
SECRET=$(kubectl get serviceaccount ${SERVICE_ACCOUNT} -n ${NAMESPACE} -o json | jq -Mr '.secrets[].name | select(contains("token"))')
TOKEN=$(kubectl get secret ${SECRET} -n ${NAMESPACE} -o json | jq -Mr '.data.token' | base64 -D)

echo "Trying to locate kind configuration..."
APISERVER=$(kind get kubeconfig | grep server | awk '{print $2}')

echo "Testing server ${APISERVER}..."
curl -s ${APISERVER}/apis/nnf.cray.hpe.com/v1alpha1/nnfdatamovements --header "Authorization: Bearer $TOKEN" --cacert ./ca.crt