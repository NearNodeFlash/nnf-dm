#include <stdio.h>
#include <copy-offload.h>

int main(void) {
    COPY_OFFLOAD *handle;

    handle = copy_offload_init();
    if (!handle) {
        fprintf(stderr, "Failed to initialize handle\n");
        return 1;
    }

    /* Configure and use the handle... */

    copy_offload_cleanup(handle);
    return 0;
}
