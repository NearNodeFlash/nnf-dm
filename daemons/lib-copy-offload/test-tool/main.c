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

/*
 *  test tool: A simple tool to use with the copy-offload library.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <curl/curl.h>

#include "../copy-offload.h"

/*
 * Print the usage of the current command line tool
 */
void usage(const char **argv) {
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -l <server_ip>:<server_port>\n", argv[0]);
    fprintf(stderr, "    -l            List all active copy-offload requests.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -c JOB_NAME <server_ip>:<server_port>\n", argv[0]);
    fprintf(stderr, "    -c JOB_NAME   Cancel the specified copy-offload request.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Usage: %s [COMMON_ARGS] -o <-C> <-W> <-S> <-D> <server_ip>:<server_port>\n", argv[0]);
    fprintf(stderr, "    -o            Perform a copy-offload request, using the following args:\n");
    fprintf(stderr, "       -C COMPUTE_NAME    Name of the local compute node.\n");
    fprintf(stderr, "       -W WORKFLOW_NAME   Name of the associated Workflow.\n");
    fprintf(stderr, "       -S SOURCE_PATH     Local path to source file to be copied.\n");
    fprintf(stderr, "       -D DEST_PATH       Local path to destination.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "COMMON_ARGS\n");
    fprintf(stderr, "    -v                  Request verbose output from this tool.\n");
    fprintf(stderr, "    -V                  Request verbose output from libcurl.\n");
    fprintf(stderr, "    -s                  Skip TLS configuration.\n");
    fprintf(stderr, "    -x CERT_FILE        CA/Server certificate file. A self-signed certificate.\n");
    fprintf(stderr, "    -y KEY_FILE         CA/Server key file.\n");
    fprintf(stderr, "    -z CLIENTCERT_FILE  CA/Server key file.\n");


}

/*
 * Main routine
 */
int main(int argc, const char **argv) {
    COPY_OFFLOAD *offload;
    char *host_and_port;
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
    char *workflow_name = NULL;
    char *source_path = NULL;
    char *dest_path = NULL;
    char *cacert_path = NULL; /* CA/server cert - a self-signed certficate */
    char *cakey_path = NULL;
    char *clientcert_path = NULL;
    int skip_tls = 0;
    int ret;

    while ((c = getopt(argc, cargv, "hvVlsx:y:z:c:oC:W:S:D:")) != -1) {
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
            case 'W':
                workflow_name = optarg;
                break;
            case 'S':
                source_path = optarg;
                break;
            case 'D':
                dest_path = optarg;
                break;
            case 'x':
                cacert_path = optarg;
                break;
            case 'y':
                cakey_path = optarg;
                break;
            case 'z':
                clientcert_path = optarg;
                break;
            default:
                usage(argv);
                exit(1);
        }
    }

    if (optind == argc - 1) {
        host_and_port = (char *)(argv[optind]);
    } else {
        usage(argv);
        exit(1);
    }
    if (o_opt) {
        if (compute_name == NULL || workflow_name == NULL || source_path == NULL || dest_path == NULL) {
            usage(argv);
            exit(1);
        }
    }

    offload = copy_offload_init();
    copy_offload_configure(offload, &host_and_port, skip_tls, cacert_path, cakey_path, clientcert_path);
    if (verbose_libcurl) {
        copy_offload_verbose(offload);
    }

    char *output = NULL;

    if (l_opt) {
        ret = copy_offload_list(offload, &output);
    } else if (c_opt) {
        ret = copy_offload_cancel(offload, job_name, &output);
    } else if (o_opt) {
        ret = copy_offload_copy(offload, compute_name, workflow_name, source_path, dest_path, &output);
    } else {
        fprintf(stderr, "What action?\n");
        copy_offload_cleanup(offload);
        exit(1);
    }

    if (output != NULL) {
        printf("%s\n", output);
        free(output);
        output = NULL;
    }

    if (verbose) {
        printf("ret %d, http_code %ld\n", ret, offload->http_code);
    }
    if (ret) {
        fprintf(stderr, "%s", offload->err_message);
    }

    copy_offload_cleanup(offload);
    exit(ret);
}
