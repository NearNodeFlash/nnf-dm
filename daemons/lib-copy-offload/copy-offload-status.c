/*
 * Copyright 2025 Hewlett Packard Enterprise Development LP
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

#include <stdio.h>
#include "copy-offload-status.h"

copy_offload_status_response_t *_copy_offload_status_parse(char *json_text) {

    copy_offload_status_response_t *response = (copy_offload_status_response_t *)calloc(1, sizeof(struct copy_offload_status_response_s));
    json_object *root = json_tokener_parse(json_text);

    // Used for memory management. All of the @response field pointers are
    // pointing into this object.
    response->_root = root;
    
    //printf("The json string:\n\n%s\n\n", json_object_to_json_string(root));
    
    json_object *object;
    if (json_object_object_get_ex(root, "state", &object)) {
        if (object != NULL) {
            response->state = json_object_get_string(object);
        }
    }
    if (json_object_object_get_ex(root, "status", &object)) {
        if (object != NULL) {
            response->status = json_object_get_string(object);
        }
    }
    if (json_object_object_get_ex(root, "message", &object)) {
        if (object != NULL) {
            response->message = json_object_get_string(object);
        }
    }
    if (json_object_object_get_ex(root, "startTime", &object)) {
        if (object != NULL) {
            response->start_time = json_object_get_string(object);
        }
    }
    if (json_object_object_get_ex(root, "endTime", &object)) {
        if (object != NULL) {
            response->end_time = json_object_get_string(object);
        }
    }

    json_object *cmd_object;
    if (json_object_object_get_ex(root, "commandStatus", &cmd_object)) {
        if (json_object_object_get_ex(cmd_object, "command", &object)) {
            if (object != NULL) {
                response->command_status.command = json_object_get_string(object);
            }
        }
        if (json_object_object_get_ex(cmd_object, "progress", &object)) {
            if (object != NULL) {
                response->command_status.progress = json_object_get_int(object);
            }
        }
        if (json_object_object_get_ex(cmd_object, "elapsedTime", &object)) {
            if (object != NULL) {
                response->command_status.elapsed_time = json_object_get_string(object);
            }
        }
        if (json_object_object_get_ex(cmd_object, "lastMessage", &object)) {
            if (object != NULL) {
                response->command_status.last_message = json_object_get_string(object);
            }
        }
        if (json_object_object_get_ex(cmd_object, "lastMessageTime", &object)) {
            if (object != NULL) {
                response->command_status.last_message_time = json_object_get_string(object);
            }
        }
    }

    return response;
}

/* Pretty-print the response record to the given file descriptor.
 */
void copy_offload_status_pretty_print(FILE *out, copy_offload_status_response_t *response) {
    if (response == NULL || response->_root == NULL) {
        fprintf(out, "The response object is empty.\n");
        return;
    }

    fprintf(out,
        "state: %s\n"
        "status: %s\n"
        "message: %s\n"
        "command: %s\n"
        "progress: %d\n"
        "elapsed time: %s\n"
        "last message: %s\n"
        "last message time: %s\n"
        "start time: %s\n"
        "end time: %s\n",
        response->state, response->status, response->message,
        response->command_status.command, response->command_status.progress,
        response->command_status.elapsed_time, response->command_status.last_message,
        response->command_status.last_message_time,
        response->start_time, response->end_time);
}

/* Cleanup the memory behind the @reponse object and its backing json object.
 * After this has been called it is no longer safe to reference anything in
 * the @response object.
 */
void copy_offload_status_cleanup(copy_offload_status_response_t *response) {
    if (response != NULL && response->_root != NULL) {
        json_object_put(response->_root);
        response->_root = NULL;
    }
}
