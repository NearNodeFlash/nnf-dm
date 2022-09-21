/*
 * Copyright 2022 Hewlett Packard Enterprise Development LP
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

#include <string>
#include <memory>
#include <vector>

namespace near_node_flash {
    
namespace data_movement {


class RPCStatus;
class Workflow;

class CreateRequest;
class CreateResponse;
class StatusRequest;
class StatusResponse;
class CancelRequest;
class CancelResponse;
class DeleteRequest;
class DeleteResponse;
class ListRequest;
class ListResponse;

class DataMoverClient {
    public:
        
        DataMoverClient(const std::string &target);
        ~DataMoverClient();

        RPCStatus Create(const Workflow &workflow, const CreateRequest &request, CreateResponse *response);
        RPCStatus Status(const Workflow &workflow, const StatusRequest &request, StatusResponse *response);
        RPCStatus Cancel(const Workflow &workflow, const CancelRequest &request, CancelResponse *response);
        RPCStatus Delete(const Workflow &workflow, const DeleteRequest &request, DeleteResponse *response);
        RPCStatus List  (const Workflow &workflow, const ListRequest &request, ListResponse *response);

    private:
        void *data_;
};

class RPCStatus {
    public:
        RPCStatus(bool ok, int error_code, std::string error_message);

        bool ok() const { return ok_; }
        int error_code() const { return error_code_; }
        std::string error_message() const { return error_message_; }

    private:
        bool ok_;
        int error_code_;
        std::string error_message_;
};

class Workflow {
    public:
        Workflow(std::string name, std::string namespace_);

    private:
        friend DataMoverClient;

        std::string name_;
        std::string namespace_;
};


class CreateRequest {
    public:
        CreateRequest(std::string source, std::string destination);
        ~CreateRequest();

    private:
        friend DataMoverClient;

        void *data_;
};

class CreateResponse {
    public:
        CreateResponse();
        ~CreateResponse();

        enum Status {
            STATUS_SUCCESS = 0,
            STATUS_FAILED = 1,
            STATUS_INVALID =2,
        };
        
        std::string     uid();
        Status          status();
        std::string     message();

    private:
        friend DataMoverClient;

        void *data_;
};

class StatusRequest {
    public:
        StatusRequest(std::string uid, int64_t maxWaitTime);
        ~StatusRequest();

    private:
        friend DataMoverClient;

        void *data_;
};

class StatusResponse {
    public:
        StatusResponse();
        ~StatusResponse();

        enum State {
            STATE_PENDING = 0,
            STATE_STARTING = 1,
            STATE_RUNNING = 2,
            STATE_COMPLETED = 3,
            STATE_UNKNOWN = 4,
        };

        enum Status {
            STATUS_INVALID = 0,
            STATUS_NOT_FOUND = 1,
            STATUS_SUCCESS = 2,
            STATUS_FAILED = 3,
            STATUS_CANCELLED = 4,
            STATUS_UNKNOWN = 5,
        };

        State           state();
        Status          status();
        std::string     message();

    private:
        friend DataMoverClient;
        
        void *data_;
};

class CancelRequest {
    public:
        CancelRequest(std::string uid);
        ~CancelRequest();

    private:
        friend DataMoverClient;

        void *data_;
};

class CancelResponse {
    public:
        CancelResponse();
        ~CancelResponse();

        enum Status {
            STATUS_INVALID = 0,
            STATUS_NOT_FOUND = 1,
            STATUS_SUCCESS = 2,
            STATUS_FAILED = 3,
        };

        Status status();
        std::string message();

    private:
        friend DataMoverClient;

        void *data_;
};

class DeleteRequest {
    public:
        DeleteRequest(std::string uid);
        ~DeleteRequest();

    private:
        friend DataMoverClient;

        void *data_;
};

class DeleteResponse {
    public:
        DeleteResponse();
        ~DeleteResponse();

        enum Status {
            STATUS_INVALID = 0,
            STATUS_NOT_FOUND = 1,
            STATUS_SUCCESS = 2,
            STATUS_FAILED = 3,
        };

        Status          status();
        std::string     message();

    private:
        friend DataMoverClient;

        void *data_;
};

class ListRequest {
    public:
        ListRequest();
        ~ListRequest();
    
    private:
        friend DataMoverClient;

        void *data_;
};

class ListResponse {
    public:
        ListResponse();
        ~ListResponse();

        std::vector<std::string> uids();

    private:
        friend DataMoverClient;

        void *data_;
};

} // namespace data_movement

} // namespace near_node_flash
