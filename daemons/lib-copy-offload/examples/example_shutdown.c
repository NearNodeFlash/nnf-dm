#include <stdio.h>
#include <stdlib.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;
    int ret;

    handle = copy_offload_init();
    if (!handle) return 1;

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

    /* Shutdown; the function checks for active jobs internally */
    ret = copy_offload_shutdown(handle);
    if (ret != 0) {
        fprintf(stderr, "Shutdown failed: %s\n",
                handle->err_message);
        copy_offload_cleanup(handle);
        return 1;
    }

    printf("Server shutdown initiated successfully\n");
    copy_offload_cleanup(handle);
    return 0;
}
