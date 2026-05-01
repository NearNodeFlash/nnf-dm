#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;
    char *job_list = NULL;
    char *job, *saveptr;
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

    ret = copy_offload_list(handle, &job_list);
    if (ret != 0) {
        fprintf(stderr, "List failed: %s\n",
                handle->err_message);
        copy_offload_cleanup(handle);
        return 1;
    }

    if (job_list != NULL && strlen(job_list) > 0) {
        printf("Active jobs:\n");
        job = strtok_r(job_list, ",", &saveptr);
        while (job != NULL) {
            printf("  %s\n", job);
            job = strtok_r(NULL, ",", &saveptr);
        }
    } else {
        printf("No active jobs\n");
    }

    free(job_list);
    copy_offload_cleanup(handle);
    return 0;
}
