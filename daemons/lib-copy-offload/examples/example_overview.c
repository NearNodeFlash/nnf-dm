#include <stdio.h>
#include <stdlib.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;
    char *job_name = NULL;
    int ret;

    /* Initialize and configure */
    handle = copy_offload_init();
    if (!handle) {
        fprintf(stderr, "Failed to initialize\n");
        return 1;
    }

    if (getenv("COPY_OFFLOAD_SKIP_TLS"))
        ret = copy_offload_configure_without_tls(handle);
    else
        ret = copy_offload_configure(handle);
    if (ret != 0) {
        fprintf(stderr, "Configure failed: %s\n",
                handle->err_message);
        copy_offload_cleanup(handle);
        return 1;
    }

    if (getenv("COPY_OFFLOAD_CERT"))
        copy_offload_override_cert(handle, getenv("COPY_OFFLOAD_CERT"));
    if (getenv("COPY_OFFLOAD_TOKEN"))
        copy_offload_override_token(handle, getenv("COPY_OFFLOAD_TOKEN"));

    /* Submit copy request */
    ret = copy_offload_copy(handle, "default", 4, 8, 0,
                            "/source/data", "/dest/data",
                            &job_name);
    if (ret != 0) {
        fprintf(stderr, "Copy failed: %s\n",
                handle->err_message);
        copy_offload_cleanup(handle);
        return 1;
    }

    printf("Job submitted: %s\n", job_name);

    /* Monitor status */
    copy_offload_status_response_t *status = NULL;
    ret = copy_offload_status(handle, job_name, 300, &status);
    if (ret == 0 && status) {
        copy_offload_status_pretty_print(stdout, status);
        copy_offload_status_cleanup(status);
    }

    free(job_name);
    copy_offload_cleanup(handle);
    return 0;
}
