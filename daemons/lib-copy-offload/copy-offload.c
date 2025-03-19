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

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include "copy-offload.h"

#define COPY_OFFLOAD_URL_SIZE 1024
#define COPY_OFFLOAD_POST_SIZE 2048
#define COPY_OFFLOAD_MSG_SIZE CURL_ERROR_SIZE * 2

struct memory {
    char *response;
    size_t size;
};

/* Read a file into a buffer. The caller is responsible for calling free() on the
 * returned buffer.
 * Returns 0 on success.
 * On failure, returns 1 and places the error message in @offload->err_message.
 */
static int read_contents(COPY_OFFLOAD *offload, char *path, char **buffer) {
    int ret = 0;
    FILE *fp;
    struct stat stat_blk;
    int cnt;
    *buffer = NULL;

    if (stat(path, &stat_blk) == -1) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Unable to stat %s: errno %d\n", path, errno);
        return 1;
    }
    if ((*buffer = (char *)malloc(stat_blk.st_size + 1)) == NULL) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Unable to allocate a buffer for the bearer token\n");
        return 1;
    }
    fp = fopen(path, "r");
    if (fp == NULL) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Unable to open %s: errno %d\n", path, errno);
        free(*buffer);
        *buffer = NULL;
        return 1;
    }
    cnt = fread(*buffer, 1, stat_blk.st_size, fp);
    if (cnt == -1) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Unable to read %s: errno %d\n", path, errno);
        free(*buffer);
        buffer = NULL;
        ret = 1;
    } else if (cnt != stat_blk.st_size) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Incomplete read of %s. Wanted %lld, got %d\n", path, (long long)stat_blk.st_size, cnt);
        free(*buffer);
        buffer = NULL;
        ret = 1;
    } else {
        (*buffer)[cnt] = '\0';
        if (cnt > 0 && ((*buffer)[cnt - 1] == '\n' || (*buffer)[cnt - 1] == '\r')) {
            (*buffer)[cnt - 1] = '\0';
        }
    }
    fclose(fp);

    return ret;
}

/* See the example in CURLOPT_WRITEFUNCTION(3). */
static size_t cb(void *data, size_t size, size_t nmemb, void *clientp) {
    size_t realsize = size * nmemb;
    struct memory *mem = (struct memory *)clientp;

    char *ptr = (char *)realloc(mem->response, mem->size + realsize + 1);
    if(!ptr) {
        return 0;
    }

    mem->response = ptr;
    memcpy(&(mem->response[mem->size]), data, realsize);
    mem->size += realsize;
    mem->response[mem->size] = 0;

    return realsize;
}

/* Create and initialize a handle. */
COPY_OFFLOAD *copy_offload_init() {
    CURL *curl;
    COPY_OFFLOAD *offload = (COPY_OFFLOAD *)malloc(sizeof(struct copy_offload_s));

    curl_global_init(CURL_GLOBAL_ALL);
    curl = curl_easy_init();
    if (!curl) {
        fprintf(stderr, "copy_offload_init failed in curl_easy_init()\n");
        return NULL;
    }
    offload->curl = curl;
    offload->host_and_port = NULL;

    return offload;
}

/* Store the host-and-port in the handle and set the basic configuration
 * for the handle.
 * If @skip_tls is set, then TLS will not be enabled.
 */
int copy_offload_configure(COPY_OFFLOAD *offload, char **host_and_port, int skip_tls) {
    int ret = 0;

    CURL *curl = offload->curl;
    offload->cert_and_token_done = 0;
    offload->host_and_port = host_and_port;
    offload->skip_tls = skip_tls;
    offload->cacert = CERT_PATH;
    strncpy(offload->proto, "http", sizeof(offload->proto));

    struct curl_slist *chunk = NULL;
    chunk = curl_slist_append(chunk, COPY_OFFLOAD_API_VERSION);
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, chunk);

    curl_easy_setopt(curl, CURLOPT_TRANSFERTEXT, 1L);
    if (!skip_tls) {
        strncpy(offload->proto, "https", sizeof(offload->proto));

        curl_easy_setopt(curl, CURLOPT_HTTP_VERSION, CURL_HTTP_VERSION_2TLS);
        curl_easy_setopt(curl, CURLOPT_SSLVERSION, CURL_SSLVERSION_TLSv1_3);
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYPEER, 1L);
        //curl_easy_setopt(curl, CURLOPT_SSL_VERIFYSTATUS, 1L);
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYHOST, 1L);

        curl_easy_setopt(curl, CURLOPT_CAPATH, NULL);
    }

    offload->token = getenv(WORKFLOW_TOKEN_ENV);

    return ret;
}

/* Override the certificate file path. */
int copy_offload_override_cert(COPY_OFFLOAD *offload, char *cert_path) {
    if (offload->cert_and_token_done) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Too late to override certificate");
        return 1;
    }
    if (cert_path == NULL) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "NULL certificate path");
        return 1;
    }

    offload->cacert = cert_path;
    return 0;
}

/* Override the token. */
int copy_offload_override_token(COPY_OFFLOAD *offload, char *token_path) {
    if (offload->cert_and_token_done) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE-1, "Too late to override token");
        return 1;
    }
    if (token_path == NULL) {
        if (offload->token != NULL)
            free(offload->token);
        offload->token = NULL;
        return 0;
    }

    char *token = NULL;
    int ret;
    ret = read_contents(offload, token_path, &token);
    if (ret != 0)
        return ret;
    if (offload->token != NULL)
        free(offload->token);
    offload->token = token;
    return 0;
}

static void copy_offload_setup_cert_and_token(COPY_OFFLOAD *offload) {
    if (offload->cert_and_token_done)
        return;
    if (!offload->skip_tls) {
        curl_easy_setopt(offload->curl, CURLOPT_CAINFO, offload->cacert);
    }
    if (offload->token != NULL) {
        curl_easy_setopt(offload->curl, CURLOPT_XOAUTH2_BEARER, offload->token);
        curl_easy_setopt(offload->curl, CURLOPT_HTTPAUTH, CURLAUTH_BEARER);
    }
    offload->cert_and_token_done = 1;
}

/* Reset the handle so it can be used for the next command.
 * After this, the handle is ready for things like the following:
 *  copy_offload_list(), copy_offload_cancel(), copy_offload_docopy().
 */
void copy_offload_reset(COPY_OFFLOAD *offload) {
    curl_easy_reset(offload->curl);
    if (offload->token != NULL)
        free(offload->token);
    offload->cert_and_token_done = 0;
    copy_offload_configure(offload, offload->host_and_port, offload->skip_tls);
}

/* Request verbose output from libcurl. */
void copy_offload_verbose(COPY_OFFLOAD *offload) {
    curl_easy_setopt(offload->curl, CURLOPT_VERBOSE, 1L);
}

/* Perform a blocking transfer.
 * Returns -1 if there was a libcurl error, otherwise returns the http response
 * status code.
 */
static long copy_offload_perform(COPY_OFFLOAD *offload, struct memory *chunk) {
    CURL *curl = offload->curl;
    CURLcode res;
    long http_code = -1;
    char errbuf[CURL_ERROR_SIZE];

    copy_offload_setup_cert_and_token(offload);
    /* Send all data to the cb() function.
     * See the example in CURLOPT_WRITEFUNCTION(3).
     */
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, cb);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, (void *)chunk);

    curl_easy_setopt(curl, CURLOPT_ERRORBUFFER, errbuf);
    errbuf[0] = 0;
    offload->err_message[0] = 0;
    offload->http_code = -1;
    res = curl_easy_perform(curl);
    if(res != CURLE_OK) {
        // Things that are not HTTP response status codes.
        // See libcurl-errors(3) and the example in CURLOPT_ERRORBUFFER(3).
        size_t len = strlen(errbuf);
        if (len)
            snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE, "libcurl: (%d) %s%s", res, errbuf, ((errbuf[len - 1] != '\n') ? "\n" : ""));
        else
            snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE, "libcurl: (%d) %s\n", res, curl_easy_strerror(res));
    } else {
        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
        offload->http_code = http_code;
    }

    return http_code;
}

/* Chop the newline from the end of the string. */
static void chop(char **output) {
    if (*output != NULL) {
        char *newline = strchr(*output, '\n');
        *newline = '\0';
    }
}

/* List the active copy-offload requests.
 * The caller is responsible for calling free() on @output if *output is non-NULL.
 */
int copy_offload_list(COPY_OFFLOAD *offload, char **output) {
    long http_code;
    struct memory chunk = {NULL, 0};
    int ret = 1;
    char urlbuf[COPY_OFFLOAD_URL_SIZE];

    snprintf(urlbuf, COPY_OFFLOAD_URL_SIZE, "%s://%s/list", offload->proto, *offload->host_and_port);
    curl_easy_setopt(offload->curl, CURLOPT_URL, urlbuf);

    http_code = copy_offload_perform(offload, &chunk);
    if (http_code == 200)
        ret = 0;
    if (chunk.response != NULL) {
        *output = strdup(chunk.response);
        chop(output);
        free(chunk.response);
    }
    return ret;
}

/* Cancel a specific copy-offload request. */
int copy_offload_cancel(COPY_OFFLOAD *offload, char *job_name, char **output) {
    long http_code;
    struct memory chunk = {NULL, 0};
    int ret = 1;
    char urlbuf[COPY_OFFLOAD_URL_SIZE];

    snprintf(urlbuf, COPY_OFFLOAD_URL_SIZE, "%s://%s/cancel/%s", offload->proto, *offload->host_and_port, job_name);
    curl_easy_setopt(offload->curl, CURLOPT_URL, urlbuf);
    curl_easy_setopt(offload->curl, CURLOPT_CUSTOMREQUEST, "DELETE");

    http_code = copy_offload_perform(offload, &chunk);
    if (http_code == 204)
        ret = 0;
    if (chunk.response != NULL) {
        *output = strdup(chunk.response);
        chop(output);
        free(chunk.response);
    }
    return ret;
}

/* Submit a new copy-offload request.
 * The caller is responsible for calling free() on @output if *output is non-NULL.
 */
int copy_offload_copy(COPY_OFFLOAD *offload, char *compute_name, char *workflow_name, const char *profile_name, int slots, int max_slots, int dry_run, char *source_path, char *dest_path, char **output) {
    long http_code;
    struct memory chunk = {NULL, 0};
    int ret = 1;
    int n;
    char urlbuf[COPY_OFFLOAD_URL_SIZE];
    char postbuf[COPY_OFFLOAD_POST_SIZE];
    char dry_run_str[8];

    snprintf(urlbuf, COPY_OFFLOAD_URL_SIZE, "%s://%s/trial", offload->proto, *offload->host_and_port);
    curl_easy_setopt(offload->curl, CURLOPT_URL, urlbuf);

    if (profile_name == NULL) {
        profile_name = "";
    }

    if (dry_run) {
        strcpy(dry_run_str, "true");
    } else {
        strcpy(dry_run_str, "false");
    }

    const char *offload_req =
        "{\"computeName\": \"%s\", "
        "\"workflowName\": \"%s\", "
        "\"sourcePath\": \"%s\", "
        "\"destinationPath\": \"%s\", "
        "\"dmProfile\": \"%s\", "
        "\"dryrun\": %s, "
        "\"dcpOptions\": \"\", "
        "\"logStdout\": true, "
        "\"storeStdout\": false, "
        "\"slots\": %d, "
        "\"maxSlots\": %d}";
    n = snprintf(postbuf, COPY_OFFLOAD_POST_SIZE, offload_req, compute_name, workflow_name, source_path, dest_path, profile_name, dry_run_str, slots, max_slots);
    if (n >= (int)sizeof(postbuf)) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE, "Error formatting request: request truncated, buffer too small");
        return ret;
    } else if (n < 0) {
        snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE, "Error formatting request");
        return ret;
    }
    curl_easy_setopt(offload->curl, CURLOPT_POSTFIELDS, postbuf);

    http_code = copy_offload_perform(offload, &chunk);
    if (chunk.response != NULL) {
        if (http_code == 200) {
            char *delim = strchr(chunk.response, '=');
            if (delim != NULL) {
                *output = strdup(delim+1);
                chop(output);
            }
            ret = 0;
        } else if (http_code != -1) {
            snprintf(offload->err_message, COPY_OFFLOAD_MSG_SIZE, "%s", chunk.response);
        }
        free(chunk.response);
    }
    return ret;
}

/* Clean up the handle's resources. */
void copy_offload_cleanup(COPY_OFFLOAD *offload) {
    curl_easy_cleanup(offload->curl);
    curl_global_cleanup();
    offload->curl = NULL;
}
