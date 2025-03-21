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

#include <curl/curl.h>

// Define the API version. This applies to the request format sent by the client
// as well as the response format sent by the server.
// See validateVersion() in the server.
#define COPY_OFFLOAD_API_VERSION "Accepts-version: 1.0"

#define COPY_OFFLOAD_MSG_SIZE CURL_ERROR_SIZE * 2

// The name of the environment variable that contains the workflow's token.
#define WORKFLOW_TOKEN_ENV "DW_WORKFLOW_TOKEN"

// The name of the environment variable that contains the workflow's name.
#define WORKFLOW_NAME_ENV "DW_WORKFLOW_NAME"

// The name of the environment variable that contains the workflow's namespace.
#define WORKFLOW_NAMESPACE_ENV "DW_WORKFLOW_NAMESPACE"

// The name of the environment variable that contains the name of the rabbit
// that is running the MPI launcher, when MPI is being used.
#define NNF_CONTAINER_LAUNCHER_ENV "NNF_CONTAINER_LAUNCHER"

// The name of the environment variable that contains the ports that the
// container is listening on. This library will use the first port in the list.
#define NNF_CONTAINER_PORTS_ENV "NNF_CONTAINER_PORTS"

// Path of the file that contains the name of the rabbit that is associated
// with the current node. This will be used when NNF_CONTAINER_LAUNCHER_ENV
// is not found, in which case the copy-offload server is expected to be on
// this rabbit.
#define LOCAL_RABBIT_CONF "/etc/local-rabbit.conf"

// Default path of the TLS certificate. This should be a text file holding the
// PEM form of the certificate.
#define CERT_PATH "/etc/nnf-dm-usercontainer/cert.pem"

struct copy_offload_s {
    CURL *curl;
    int skip_tls;
    int cert_and_token_done;
    char *cacert;
    char *token;
    char *token_buf;
    char proto[6];
    char my_host_name[256];
    char *workflow_name;
    char *workflow_namespace;
    char *server_host;
    char *server_host_buf;
    char *server_port;

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

/* Setup the handle with information needed to connect to the server and with
 * TLS enabled.
 * Returns 0 on success.
 * On failure, returns 1 and places the error message in @offload->err_message. 
 */
int copy_offload_configure(COPY_OFFLOAD *offload);

/* Submit a new copy-offload request.
 * The new job's name will be placed in @output. This name is used to cancel the
 * request or to check its status.
 * The caller is responsible for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_copy(COPY_OFFLOAD *offload, const char *profile_name, int slots, int max_slots, int dry_run, char *source_path, char *dest_path, char **output);

/* List the active copy-offload requests.
 * The comma-separated list of job names will be placed in @output. The job names
 * are the same ones returned by copy_offload_copy().
 * The caller is responsible for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_list(COPY_OFFLOAD *offload, char **output);

/* Submit a status request. The @job_name is the value returned by copy_offload_copy().
 * The caller is responsible for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message. 
 */
int copy_offload_status(COPY_OFFLOAD *offload, char *job_name, int max_wait_secs, char **output);

/* Cancel a specific copy-offload request. The @job_name is the value returned
 * by copy_offload_copy().
 * Any output from the server, if present, will be placed in @output. The caller
 * is responsible for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_cancel(COPY_OFFLOAD *offload, char *job_name, char **output);

/* Clean up the handle's resources. */
void copy_offload_cleanup(COPY_OFFLOAD *offload);

/*
 * APIs used for development or debugging.
 */

/* Request verbose output from libcurl. */
void copy_offload_verbose(COPY_OFFLOAD *offload);

/* Send a hello message to the server.
 * The server's response will be placed in @output. The caller is responsible
 * for calling free() on @output if *output is non-NULL.
 * Returns 0 on success.
 * On failure it returns 1 and places an error message in @offload->err_message.
 */
int copy_offload_hello(COPY_OFFLOAD *offload, char **output);

/* Setup the handle with information needed to connect to the server but with
 * TLS disabled.
 * Returns 0 on success.
 * On failure, returns 1 and places the error message in @offload->err_message. 
 */
int copy_offload_configure_without_tls(COPY_OFFLOAD *offload);

/* Override the default certificate path. By default, the certificate will be
 * read from CERT_PATH, as set by the administrator. This may be overridden by
 * placing the PEM form of the certificate in the @cert_path file.
 * This may be called any time before the first message is sent to the server.
 * Returns 0 on success.
 * On failure, returns 1 and places the error message in @offload->err_message. 
 */
int copy_offload_override_cert(COPY_OFFLOAD *offload, char *cert_path);

/* Override the default token. By default, the token will be found in the
 * $DW_WORKFLOW_TOKEN environment variable.
 * Set @token_path to NULL to not use a token, or set it to a file that contains
 * the token if not using the environment variable.
 * This may be called any time before the first message is sent to the server.
 * Returns 0 on success.
 * On failure, returns 1 and places the error message in @offload->err_message. 
 */
int copy_offload_override_token(COPY_OFFLOAD *offload, char *token_path);

/* Override the local hostname.
 * By default, this is gethostname(2).
 */
int copy_offload_override_hostname(COPY_OFFLOAD *offload, char *hostname);
