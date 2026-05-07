#include <stdio.h>
#include <stdlib.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;
    char *job_name = NULL;
    copy_offload_status_response_t *status = NULL;
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
                            "/source/data", "/dest/data",
                            &job_name);
    if (ret != 0) {
        fprintf(stderr, "Copy failed: %s\n",
                handle->err_message);
        goto cleanup;
    }

    /* Wait up to 5 minutes for completion */
    ret = copy_offload_status(handle, job_name, 300, &status);
    if (ret != 0) {
        fprintf(stderr, "Status query failed: %s\n",
                handle->err_message);
    } else if (status) {
        printf("Job state: %s\n", status->state);
        printf("Progress: %d%%\n", status->command_status.progress);

        /* Pretty-print full status */
        copy_offload_status_pretty_print(stdout, status);

        copy_offload_status_cleanup(status);
    }
    ret = 0;

cleanup:
    free(job_name);
    copy_offload_cleanup(handle);
    return ret;
}
