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

CC ?= /usr/bin/gcc
AR ?= /usr/bin/ar
CFLAGS = -Wall -Werror -g -I/opt/homebrew/include -I/opt/homebrew/opt/curl/include
LDFLAGS = -lcurl -L/opt/homebrew/lib -L/opt/homebrew/opt/curl/lib
ARFLAGS = rcs

all: libcopyoffload.a tester

libcopyoffload.a: copy-offload.o
	$(AR) $(ARFLAGS) libcopyoffload.a copy-offload.o

tester: test-tool/main.o libcopyoffload.a
	$(CC) $(CFLAGS) -o tester test-tool/main.o -L. -lcopyoffload $(LDFLAGS)

test-tool/main.o: test-tool/main.c copy-offload.h
copy-offload.o: copy-offload.c copy-offload.h

clean:
	rm -f *.o core tester *.a
	rm -f test-tool/*.o
