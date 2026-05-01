#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;
    char *job_name = NULL;
    char *response = NULL;
    int ret = 1;

    handle = copy_offload_init();
    if (!handle) {
        fprintf(stderr, "Init failed\n");
        return 1;
    }

    if (getenv("COPY_OFFLOAD_SKIP_TLS"))
        ret = copy_offload_configure_without_tls(handle);
    else
        ret = copy_offload_configure(handle);
    if (ret != 0) {
        fprintf(stderr, "Configure failed: %s\n",
                handle->err_message);
        goto cleanup;
    }

    if (getenv("COPY_OFFLOAD_CERT"))
        copy_offload_override_cert(handle, getenv("COPY_OFFLOAD_CERT"));
    if (getenv("COPY_OFFLOAD_TOKEN"))
        copy_offload_override_token(handle, getenv("COPY_OFFLOAD_TOKEN"));

    /* Submit a job */
    ret = copy_offload_copy(handle, NULL, 4, 8, 0,
                            "/source/large-data",
                            "/dest/large-data",
                            &job_name);
    if (ret != 0) {
        fprintf(stderr, "Copy submission failed: %s\n",
                handle->err_message);
        goto cleanup;
    }

    printf("Job submitted: %s\n", job_name);
    sleep(2);  /* Let it start */

    /* Cancel the job */
    ret = copy_offload_cancel(handle, job_name, &response);
    if (ret != 0) {
        fprintf(stderr, "Cancel failed: %s (HTTP %ld)\n",
                handle->err_message, handle->http_code);
    } else {
        printf("Cancellation requested\n");
        if (response) {
            printf("Server response: %s\n", response);
            free(response);
        }
    }
    ret = 0;

cleanup:
    free(job_name);
    copy_offload_cleanup(handle);
    return ret;
}
