#include <stdio.h>
#include <stdarg.h> // for va_start, va_end
#include <string.h> // for strlen
#include <stdlib.h>

#include "client.h"
#include "rsyncdatamovement.pb-c.h"

void die(const char *format, ...) {
    va_list args;
    va_start (args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
    exit(1);
}

int main(int argc, char **argv) {
    char *socketAddr = "/var/run/nnf-dm.sock";

    struct OpenConnection_return conn;
    conn = OpenConnection(socketAddr);
    if (conn.r1 != 0) {
        die("Failed to open connection");
    }

    // Create a request to copy data from source file to destination file. This Create
    // implementation assumes environmental variables are defined below. If these
    // environmental variables are not defined, you might need to expand on the Create
    // functionality so they can be user supplied values.
    // - Initiator          (NODE_NAME)             Defines the name of the initiating compute node
    // - Target             (NNF_NODE_NAME)         Defines the name of the target rabbit node
    // - Workflow           (DW_WORKFLOW_NAME)      Defines the name of the owning workflow
    // - Workflow Namespace (DW_WORKFLOW_NAMESPACE) Defines the namespace of the owning workflow
    struct Create_return createResponse;
    createResponse = Create("your_source_file.in", "your_destination_file.out");
    if (createResponse.r1 != 0) {
        die("Failed to create request");
    }


    // Check the status of the data movement task.
    struct Status_return statusResponse;
    statusResponse = Status(createResponse.r0);
    if (statusResponse.r2 != 0) {
        die("Failed to retrieve status");
    }

    switch (statusResponse.r0) {
        case DATAMOVEMENT__RSYNC_DATA_MOVEMENT_STATUS_RESPONSE__STATE__PENDING:
            printf("Request Pending\n");
            break;
        case DATAMOVEMENT__RSYNC_DATA_MOVEMENT_STATUS_RESPONSE__STATE__STARTING:
            printf("Request Starting\n");
            break;
        case DATAMOVEMENT__RSYNC_DATA_MOVEMENT_STATUS_RESPONSE__STATE__RUNNING:
            printf("Request Running\n");
            break;
        case DATAMOVEMENT__RSYNC_DATA_MOVEMENT_STATUS_RESPONSE__STATE__COMPLETED:
            printf("Request Completed\n");
            break;
        default:
            printf("Request State Unknown\n");
            break;
    }

    // More information can be found by looking at r1 (status)

    // Delete the request
    struct Delete_return deleteResponse;
    deleteResponse = Delete(createResponse.r0);
    if (deleteResponse.r1 != 0) {
        die("Failed to delete");
    }

    Free(createResponse.r0);





    CloseConnection(conn.r0);
}