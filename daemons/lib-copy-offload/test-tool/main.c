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

/*
 *  test tool: A simple tool to use with the copy-offload library.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "../copy-offload.h"

/*
 * Print the usage of the current command line tool
 */
void usage(const char **argv) {
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -H\n", argv[0]);
    fprintf(stderr, "    -H            Send a hello message to the server.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -l\n", argv[0]);
    fprintf(stderr, "    -l            List all active copy-offload requests.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -c JOB_NAME\n", argv[0]);
    fprintf(stderr, "    -c JOB_NAME   Cancel the specified copy-offload request.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -o <ARGS>\n", argv[0]);
    fprintf(stderr, "    -o            Perform a copy-offload request, using the following args:\n");
    fprintf(stderr, "       -P DM_PROFILE_NAME Name of the DM profile to use (optional).\n");
    fprintf(stderr, "       -S SOURCE_PATH     Local path to source file to be copied.\n");
    fprintf(stderr, "       -D DEST_PATH       Local path to destination.\n");
    fprintf(stderr, "       -m SLOTS           Number of slots (processes).\n");
    fprintf(stderr, "       -M MAX_SLOTS       Maximum number of slots (processes).\n");
    fprintf(stderr, "       -C MY_HOSTNAME     Name of the host submitting the request. (for development/debugging)\n");
    fprintf(stderr, "       -d                 Perform a dry run.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -S <ARGS>\n", argv[0]);
    fprintf(stderr, "    -q            Submit a status request for the specified job.\n");
    fprintf(stderr, "       -j JOB_NAME    The job name returned by the copy-offload request.\n");
    fprintf(stderr, "       -w MAX_WAIT    Maximum number of seconds to wait for the job to complete. (default 0)\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -X\n", argv[0]);
    fprintf(stderr, "    -X     Shutdown the copy-offload server.\n");
    fprintf(stderr, "COMMON_ARGS\n");
    fprintf(stderr, "    -v                  Request verbose output from this tool.\n");
    fprintf(stderr, "    -V                  Request verbose output from libcurl.\n");
    fprintf(stderr, "    -s                  Skip TLS configuration.\n");
    fprintf(stderr, "    -t TOKEN_FILE       Bearer token file.\n");
    fprintf(stderr, "    -x CERT_FILE        CA/Server certificate file. A self-signed certificate.\n");
}

/*
 * Main routine
 */
int main(int argc, const char **argv) {
    COPY_OFFLOAD *offload;
    int c;
    opterr = 0;
    char * const *cargv = (char * const *)argv;
    int l_opt = 0;
    int c_opt = 0;
    int o_opt = 0;
    int verbose = 0;
    int verbose_libcurl = 0;
    char *job_name = NULL;
    char *compute_name = NULL;
    char *profile_name = NULL;
    char *source_path = NULL;
    char *dest_path = NULL;
    char *cacert_path = NULL; /* CA/server cert - a self-signed certficate */
    char *token_path = NULL;
    int dry_run = 0;
    int skip_tls = 0;
    int slots = -1; /* -1 defers to dm profile, 0 disables slots */
    int max_slots = -1;
    int H_opt = 0;
    int q_opt = 0;
    int X_opt = 0;
    int max_wait_secs = 0;
    int ret;
    copy_offload_status_response_t *status_response = NULL;

    while ((c = getopt(argc, cargv, "hvVlst:x:c:oC:P:S:D:m:M:dHqj:w:X")) != -1) {
        switch (c) {
            case 'c':
                c_opt = 1;
                job_name = optarg;
                break;
            case 'l':
                l_opt = 1;
                break;
            case 'o':
                o_opt = 1;
                break;
            case 'v':
                verbose = 1;
                break;
            case 'V':
                verbose_libcurl = 1;
                break;
            case 's':
                skip_tls = 1;
                break;
            case 'C':
                compute_name = optarg;
                break;
            case 'P':
                profile_name = optarg;
                break;
            case 'S':
                source_path = optarg;
                break;
            case 'D':
                dest_path = optarg;
                break;
            case 'm':
                slots = atoi(optarg);
                break;
            case 'M':
                max_slots = atoi(optarg);
                break;
            case 'd':
                dry_run = 1;
                break;
            case 't':
                token_path = optarg;
                break;
            case 'x':
                cacert_path = optarg;
                break;
            case 'H':
                H_opt = 1;
                break;
            case 'q':
                q_opt = 1;
                break;
            case 'j':
                job_name = optarg;
                break;
            case 'w':
                max_wait_secs = atoi(optarg);
                break;
            case 'X':
                X_opt = 1;
                break;
            default:
                usage(argv);
                exit(1);
        }
    }

    if (optind != argc) {
        usage(argv);
        exit(1);
    }
    if (o_opt) {
        if (source_path == NULL || dest_path == NULL) {
            usage(argv);
            exit(1);
        }
    }
    if (q_opt || c_opt) {
        if (job_name == NULL) {
            usage(argv);
            exit(1);
        }
    }

    offload = copy_offload_init();
    if (skip_tls) {
        ret = copy_offload_configure_without_tls(offload);
    } else {
        ret = copy_offload_configure(offload);
    }
    if (ret != 0) {
        fprintf(stderr, "%s\n", offload->err_message);
        exit(1);
    }
    if (cacert_path != NULL) {
        ret = copy_offload_override_cert(offload, cacert_path);
        if (ret != 0) {
            fprintf(stderr, "%s\n", offload->err_message);
            exit(1);
        }
    }
    if (token_path != NULL) {
        ret = copy_offload_override_token(offload, token_path);
        if (ret != 0) {
            fprintf(stderr, "%s\n", offload->err_message);
            exit(1);
        }
    }
    if (compute_name != NULL) {
        ret = copy_offload_override_hostname(offload, compute_name);
        if (ret != 0) {
            fprintf(stderr, "%s\n", offload->err_message);
            exit(1);
        }
    }
    if (verbose_libcurl) {
        copy_offload_verbose(offload);
    }

    char *output = NULL;

    if (l_opt) {
        ret = copy_offload_list(offload, &output);
    } else if (c_opt) {
        ret = copy_offload_cancel(offload, job_name, &output);
    } else if (o_opt) {
        ret = copy_offload_copy(offload, profile_name, slots, max_slots, dry_run, source_path, dest_path, &output);
    } else if (H_opt) {
        ret = copy_offload_hello(offload, &output);
    } else if (q_opt) {
        ret = copy_offload_status(offload, job_name, max_wait_secs, &status_response);
    } else if (X_opt) {
        ret = copy_offload_shutdown(offload);
    } else {
        fprintf(stderr, "What action?\n");
        copy_offload_cleanup(offload);
        exit(1);
    }

    if (output != NULL) {
        printf("%s\n", output);
        free(output);
        output = NULL;
    } else if (q_opt && status_response != NULL) {
        copy_offload_status_pretty_print(stdout, status_response);
        copy_offload_status_cleanup(status_response);
    }

    if (verbose) {
        printf("ret %d, http_code %ld\n", ret, offload->http_code);
    }
    if (ret) {
        fprintf(stderr, "%s\n", offload->err_message);
    }

    copy_offload_cleanup(offload);
    exit(ret);
}
