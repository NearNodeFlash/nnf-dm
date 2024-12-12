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

#include <curl/curl.h>

// Define the API version. This applies to the request format sent by the client
// as well as the response format sent by the server.
// See validateVersion() in the server.
#define COPY_OFFLOAD_API_VERSION "Accepts-version: 1.0"

#define COPY_OFFLOAD_MSG_SIZE CURL_ERROR_SIZE * 2

struct copy_offload_s {
    CURL *curl;
    int skip_tls;
    char **host_and_port;
    char *cacert;
    char *key;
    char *clientcert;
    char proto[6];

    /* The post-processed error message. If there was an error from libcurl, then
     * that error code and message will be in this buffer. If libcurl succeeded
     * but there was an http error from the server, then that error message
     * will be in this buffer.
     */
    char err_message[COPY_OFFLOAD_MSG_SIZE];
    /* The http response status code, if there was one. Otherwise it's -1. */
    long http_code;
};
typedef struct copy_offload_s COPY_OFFLOAD;

/* Create and initialize a handle. */
COPY_OFFLOAD *copy_offload_init();

/* Store the host-and-port in the handle and set the basic configuration
 * for the handle.
 */
void copy_offload_configure(COPY_OFFLOAD *offload, char **host_and_port, int skip_tls, char *cacert, char *key, char *clientcert);

/* Reset the handle so it can be used for the next command.
 * After this, the handle is ready for things like the following:
 *  copy_offload_list(), copy_offload_cancel(), copy_offload_docopy().
 */
void copy_offload_reset(COPY_OFFLOAD *offload);

/* Request verbose output from libcurl. */
void copy_offload_verbose(COPY_OFFLOAD *offload);

/* List the active copy-offload requests.
 * The comma-separated list of job names will be placed in @output. The caller
 * is responsible for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_list(COPY_OFFLOAD *offload, char **output);

/* Cancel a specific copy-offload request.
 * Any output from the server, if present, will be placed in @output. The caller
 * is responsible for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_cancel(COPY_OFFLOAD *offload, char *job_name, char **output);

/* Submit a new copy-offload request.
 * The new job's name will be placed in @output. The caller is responsible
 * for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_copy(COPY_OFFLOAD *offload, char *compute_name, char *workflow_name, char *source_path, char *dest_path, char **output);

/* Clean up the handle's resources. */
void copy_offload_cleanup(COPY_OFFLOAD *offload);
