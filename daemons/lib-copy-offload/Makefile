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

CC ?= /usr/bin/gcc
AR ?= /usr/bin/ar
CFLAGS = -Wall -Werror -g `pkg-config --cflags libcurl json-c`
LDFLAGS = `pkg-config --libs libcurl json-c`
ARFLAGS = rcs

.PHONY: all
all: lib-dependencies libcopyoffload.a tester

.PHONY: lib-dependencies
lib-dependencies:
	@ pkg-config --libs libcurl json-c > /dev/null || echo "On a Mac use 'brew install json-c' and on a Linux node 'dnf install json-c json-c-devel'."

libcopyoffload.a: copy-offload.o copy-offload-status.o
	$(AR) $(ARFLAGS) libcopyoffload.a copy-offload.o copy-offload-status.o

tester: test-tool/main.o libcopyoffload.a
	$(CC) $(CFLAGS) -o tester test-tool/main.o -L. -lcopyoffload $(LDFLAGS)

test-tool/main.o: test-tool/main.c copy-offload.h
copy-offload.o: copy-offload.c copy-offload.h copy-offload-status.c
copy-offload-status.o: copy-offload-status.c copy-offload.h

.PHONY: clean
clean:
	rm -f *.o core tester *.a
	rm -f test-tool/*.o
