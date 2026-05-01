#include <stdio.h>
#include <stdlib.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle = NULL;
    char *job_name = NULL;
    int ret = 1;

    handle = copy_offload_init();
    if (!handle) {
        fprintf(stderr, "Init failed\n");
        goto cleanup;
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

    if (copy_offload_copy(handle, NULL, 4, 8, 0,
                          "/source/data", "/dest/data",
                          &job_name) != 0) {
        fprintf(stderr, "Copy failed: %s\n",
                handle->err_message);
        goto cleanup;
    }

    printf("Job: %s\n", job_name);
    ret = 0;

cleanup:
    free(job_name);
    if (handle) {
        copy_offload_cleanup(handle);
    }
    /* handle is now invalid, do not use */
    return ret;
}
