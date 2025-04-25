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

#include <json-c/json.h>

/* The results as returned by copy_offload_status().
 */

// Data movement operations contain a CommandStatus in the Status object.
// This information relays the current state of the data movement progress.
struct _copy_offload_command_status_s {
    // Command used to perform Data Movement
    const char *command;
    // Progress percentage (0-100%) of the data movement based on the output of `dcp`. `dcp` must be present in the command.
    int   progress;
	// Duration of how long the operation has been running (e.g. 1m30s)
    const char *elapsed_time;
	// The most recent line of output from the data movement command
    const char *last_message;
	// The time (local) of lastMessage
    const char *last_message_time;
};

// The Data Movement Status Response message defines the current status of a
// data movement request.
// This is the v1.0 apiVersion. See COPY_OFFLOAD_API_VERSION.
struct copy_offload_status_response_s {
    json_object *_root;
    // Current state of the Data Movement Request
    const char *state;
	// Current status of the Data Movement Request
    const char *status;
	// Any extra information supplied with the state/status
    const char *message;
    // Current state/progress of the Data Movement command
    struct _copy_offload_command_status_s command_status;
	// The start time (local) of the data movement operation
    const char *start_time;
	// The end time (local) of the data movement operation
    const char *end_time;
};
typedef struct copy_offload_status_response_s copy_offload_status_response_t;

copy_offload_status_response_t *_copy_offload_status_parse(char *json_text);
