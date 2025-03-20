# Copyright 2025 Hewlett Packard Enterprise Development LP
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

all:

.PHONY: within-container-unit-test
within-container-unit-test: export SKIP_BUILD=1
within-container-unit-test: e2e-mock

.PHONY: e2e-mock
e2e-mock:
	# E2E mock test, without TLS or token
	SKIP_TLS=1 SKIP_TOKEN=1 ./daemons/copy-offload-testing/e2e-mocked.sh

	# E2E mock test, with token, without TLS
	SKIP_TLS=1 ./daemons/copy-offload-testing/e2e-mocked.sh

	# E2E mock test, with TLS, without token
	SKIP_MTLS=1 SKIP_TOKEN=1 ./daemons/copy-offload-testing/e2e-mocked.sh

	# E2E mock test, with token and TLS
	./daemons/copy-offload-testing/e2e-mocked.sh

