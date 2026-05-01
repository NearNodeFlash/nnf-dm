#include <stdio.h>
#include <stdlib.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;
    char *job_name = NULL;
    int ret;

    handle = copy_offload_init();
    if (!handle) return 1;

    if (getenv("COPY_OFFLOAD_SKIP_TLS"))
        ret = copy_offload_configure_without_tls(handle);
    else
        ret = copy_offload_configure(handle);
    if (ret != 0) {
        fprintf(stderr, "Configure error: %s\n",
                handle->err_message);
        copy_offload_cleanup(handle);
        return 1;
    }

    if (getenv("COPY_OFFLOAD_CERT"))
        copy_offload_override_cert(handle, getenv("COPY_OFFLOAD_CERT"));
    if (getenv("COPY_OFFLOAD_TOKEN"))
        copy_offload_override_token(handle, getenv("COPY_OFFLOAD_TOKEN"));

    /* Submit copy with 4 initial slots, up to 8 max */
    ret = copy_offload_copy(handle, "high-performance",
                            4, 8, 0,
                            "/lustre/data/input.dat",
                            "/nvme/scratch/output.dat",
                            &job_name);
    if (ret != 0) {
        fprintf(stderr, "Copy failed: %s (HTTP %ld)\n",
                handle->err_message, handle->http_code);
        copy_offload_cleanup(handle);
        return 1;
    }

    printf("Copy job submitted successfully: %s\n", job_name);

    /* Monitor or wait for completion... */

    free(job_name);
    copy_offload_cleanup(handle);
    return 0;
}
