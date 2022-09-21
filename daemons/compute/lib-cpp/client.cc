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

#include <memory>
#include <iostream>

#include <grpcpp/grpcpp.h>

#include "client.h"
#include "datamovement.grpc.pb.h"

#define CHECK_ENUM_EQUALITY(e1, e2) \
    static_assert(static_cast<int>(e1) == static_cast<int>(e2), "Mismatch " #e1)

namespace near_node_flash {

namespace data_movement {

class DataMoverClientInternal {
    public:
        DataMoverClientInternal(std::shared_ptr<grpc::Channel> channel) : stub_(datamovement::DataMover::NewStub(channel)) {}

        std::unique_ptr<datamovement::DataMover::Stub> stub_;
};

DataMoverClient::DataMoverClient(const std::string &target) {
    auto client = new DataMoverClientInternal(grpc::CreateChannel(target, grpc::InsecureChannelCredentials()));
    data_ = static_cast<void *>(client);
}

DataMoverClient::~DataMoverClient() {
    delete static_cast<DataMoverClientInternal *>(data_);
}

RPCStatus DataMoverClient::Create(const Workflow &workflow, const CreateRequest &request, CreateResponse *response) {
    auto client = static_cast<DataMoverClientInternal *>(data_);

    auto workflow_ = new datamovement::DataMovementWorkflow();
    workflow_->set_name(workflow.name_);
    workflow_->set_namespace_(workflow.namespace_);

    auto request_ = static_cast<datamovement::DataMovementCreateRequest *>(request.data_);
    request_->set_allocated_workflow(workflow_);

    grpc::ClientContext context;
    grpc::Status status = client->stub_->Create(&context, 
        *static_cast<datamovement::DataMovementCreateRequest *>(request.data_),
        static_cast<datamovement::DataMovementCreateResponse *>(response->data_)
    );

    return RPCStatus(status.ok(), status.error_code(), status.error_message());
}

RPCStatus DataMoverClient::Status(const Workflow &workflow, const StatusRequest &request, StatusResponse *response) {
    auto client = static_cast<DataMoverClientInternal *>(data_);

    auto workflow_ = new datamovement::DataMovementWorkflow();
    workflow_->set_name(workflow.name_);
    workflow_->set_namespace_(workflow.namespace_);

    auto request_ = static_cast<datamovement::DataMovementStatusRequest *>(request.data_);
    request_->set_allocated_workflow(workflow_);

    grpc::ClientContext context;
    grpc::Status status = client->stub_->Status(&context, 
        *static_cast<datamovement::DataMovementStatusRequest *>(request.data_),
        static_cast<datamovement::DataMovementStatusResponse *>(response->data_)
    );

    return RPCStatus(status.ok(), status.error_code(), status.error_message());
}

RPCStatus DataMoverClient::Cancel(const Workflow &workflow, const CancelRequest &request, CancelResponse *response) {
    auto client = static_cast<DataMoverClientInternal *>(data_);

    auto workflow_ = new datamovement::DataMovementWorkflow();
    workflow_->set_name(workflow.name_);
    workflow_->set_namespace_(workflow.namespace_);

    auto request_ = static_cast<datamovement::DataMovementCancelRequest *>(request.data_);
    request_->set_allocated_workflow(workflow_);

    grpc::ClientContext context;
    grpc::Status status = client->stub_->Cancel(&context, 
        *static_cast<datamovement::DataMovementCancelRequest *>(request.data_),
        static_cast<datamovement::DataMovementCancelResponse *>(response->data_)
    );

    return RPCStatus(status.ok(), status.error_code(), status.error_message());
}

RPCStatus DataMoverClient::Delete(const Workflow &workflow, const DeleteRequest &request, DeleteResponse *response) {
    auto client = static_cast<DataMoverClientInternal *>(data_);

    auto workflow_ = new datamovement::DataMovementWorkflow();
    workflow_->set_name(workflow.name_);
    workflow_->set_namespace_(workflow.namespace_);

    auto request_ = static_cast<datamovement::DataMovementDeleteRequest *>(request.data_);
    request_->set_allocated_workflow(workflow_);

    grpc::ClientContext context;
    grpc::Status status = client->stub_->Delete(&context, 
        *static_cast<datamovement::DataMovementDeleteRequest *>(request.data_),
        static_cast<datamovement::DataMovementDeleteResponse *>(response->data_)
    );

    return RPCStatus(status.ok(), status.error_code(), status.error_message());
}

RPCStatus DataMoverClient::List(const Workflow &workflow, const ListRequest &request, ListResponse *response) {
    auto client = static_cast<DataMoverClientInternal *>(data_);

    auto workflow_ = new datamovement::DataMovementWorkflow();
    workflow_->set_name(workflow.name_);
    workflow_->set_namespace_(workflow.namespace_);

    auto request_ = static_cast<datamovement::DataMovementListRequest *>(request.data_);
    request_->set_allocated_workflow(workflow_);

    grpc::ClientContext context;
    grpc::Status status = client->stub_->List(&context, 
        *static_cast<datamovement::DataMovementListRequest *>(request.data_),
        static_cast<datamovement::DataMovementListResponse *>(response->data_)
    );

    return RPCStatus(status.ok(), status.error_code(), status.error_message()); 
}

RPCStatus::RPCStatus(bool ok, int error_code, std::string error_message) :
    ok_(ok),
    error_code_(error_code),
    error_message_(error_message)
{ }

Workflow::Workflow(std::string name, std::string namespace_) :
    name_(name),
    namespace_(namespace_)
{ }

CreateRequest::CreateRequest(std::string source, std::string destination) {
    auto request = new datamovement::DataMovementCreateRequest();

    request->set_source(source);
    request->set_destination(destination);

    data_ = static_cast<void *>(request);
}

CreateRequest::~CreateRequest() {
    delete static_cast<datamovement::DataMovementCreateRequest *>(data_);
}

CreateResponse::CreateResponse() {
    auto response = new datamovement::DataMovementCreateResponse();
    data_ = static_cast<void *>(response);
}

CreateResponse::~CreateResponse() {
    delete static_cast<datamovement::DataMovementCreateResponse *>(data_);
}

std::string CreateResponse::uid() { 
    return static_cast<datamovement::DataMovementCreateResponse *>(data_)->uid();
}

CreateResponse::Status CreateResponse::status() {
    CHECK_ENUM_EQUALITY(CreateResponse::STATUS_SUCCESS, datamovement::DataMovementCreateResponse_Status_SUCCESS);
    CHECK_ENUM_EQUALITY(CreateResponse::STATUS_FAILED,  datamovement::DataMovementCreateResponse_Status_FAILED);
    CHECK_ENUM_EQUALITY(CreateResponse::STATUS_INVALID, datamovement::DataMovementCreateResponse_Status_INVALID);

    auto status = static_cast<datamovement::DataMovementCreateResponse *>(data_)->status();
    return static_cast<CreateResponse::Status>(status);
}

std::string CreateResponse::message() {
    return static_cast<datamovement::DataMovementCreateResponse *>(data_)->message();
}

StatusRequest::StatusRequest(std::string uid, int64_t maxWaitTime) { 
    auto request = new datamovement::DataMovementStatusRequest();
    request->set_uid(uid);
    request->set_maxwaittime(maxWaitTime);

    data_ = static_cast<void *>(request);
}

StatusRequest::~StatusRequest() {
    delete static_cast<datamovement::DataMovementStatusRequest *>(data_);
}

StatusResponse::StatusResponse() {
    auto response = new datamovement::DataMovementStatusResponse();
    data_ = static_cast<void *>(response);
}

StatusResponse::~StatusResponse() {
    delete static_cast<datamovement::DataMovementStatusResponse *>(data_);
}

StatusResponse::State StatusResponse::state() {
    CHECK_ENUM_EQUALITY(StatusResponse::STATE_PENDING,   datamovement::DataMovementStatusResponse_State_PENDING);
    CHECK_ENUM_EQUALITY(StatusResponse::STATE_STARTING,  datamovement::DataMovementStatusResponse_State_STARTING);
    CHECK_ENUM_EQUALITY(StatusResponse::STATE_RUNNING,   datamovement::DataMovementStatusResponse_State_RUNNING);
    CHECK_ENUM_EQUALITY(StatusResponse::STATE_COMPLETED, datamovement::DataMovementStatusResponse_State_COMPLETED);
    CHECK_ENUM_EQUALITY(StatusResponse::STATE_UNKNOWN,   datamovement::DataMovementStatusResponse_State_UNKNOWN_STATE);

    auto state = static_cast<datamovement::DataMovementStatusResponse *>(data_)->state();
    return static_cast<StatusResponse::State>(state);
}

StatusResponse::Status StatusResponse::status() {
    CHECK_ENUM_EQUALITY(StatusResponse::STATUS_INVALID,   datamovement::DataMovementStatusResponse_Status_INVALID);
    CHECK_ENUM_EQUALITY(StatusResponse::STATUS_NOT_FOUND, datamovement::DataMovementStatusResponse_Status_NOT_FOUND);
    CHECK_ENUM_EQUALITY(StatusResponse::STATUS_SUCCESS,   datamovement::DataMovementStatusResponse_Status_SUCCESS);
    CHECK_ENUM_EQUALITY(StatusResponse::STATUS_FAILED,    datamovement::DataMovementStatusResponse_Status_FAILED);
    CHECK_ENUM_EQUALITY(StatusResponse::STATUS_CANCELLED, datamovement::DataMovementStatusResponse_Status_CANCELLED);
    CHECK_ENUM_EQUALITY(StatusResponse::STATUS_UNKNOWN,   datamovement::DataMovementStatusResponse_Status_UNKNOWN_STATUS);

    auto status = static_cast<datamovement::DataMovementStatusResponse *>(data_)->status();
    return static_cast<StatusResponse::Status>(status);
}

std::string StatusResponse::message() {
    return static_cast<datamovement::DataMovementStatusResponse *>(data_)->message();
}

CancelRequest::CancelRequest(std::string uid) {
    auto request = new datamovement::DataMovementCancelRequest();
    request->set_uid(uid);

    data_ = static_cast<void *>(request);
}

CancelRequest::~CancelRequest() {
    delete static_cast<datamovement::DataMovementCancelRequest *>(data_);
}

CancelResponse::CancelResponse() {
    auto response = new datamovement::DataMovementCancelResponse();
    data_ = static_cast<void *>(response);
}

CancelResponse::~CancelResponse() {
    delete static_cast<datamovement::DataMovementCancelResponse *>(data_);
}

CancelResponse::Status CancelResponse::status() {
    CHECK_ENUM_EQUALITY(CancelResponse::STATUS_INVALID,   datamovement::DataMovementCancelResponse_Status_INVALID);
    CHECK_ENUM_EQUALITY(CancelResponse::STATUS_NOT_FOUND, datamovement::DataMovementCancelResponse_Status_NOT_FOUND);
    CHECK_ENUM_EQUALITY(CancelResponse::STATUS_SUCCESS,   datamovement::DataMovementCancelResponse_Status_SUCCESS);
    CHECK_ENUM_EQUALITY(CancelResponse::STATUS_FAILED,    datamovement::DataMovementCancelResponse_Status_FAILED);

    auto status = static_cast<datamovement::DataMovementCancelResponse *>(data_)->status();
    return static_cast<CancelResponse::Status>(status);
}

std::string CancelResponse::message() {
    return static_cast<datamovement::DataMovementCancelResponse *>(data_)->message();
}

DeleteRequest::DeleteRequest(std::string uid) {
    auto request = new datamovement::DataMovementDeleteRequest();
    request->set_uid(uid);

    data_ = static_cast<void *>(request);
}

DeleteRequest::~DeleteRequest() {
    delete static_cast<datamovement::DataMovementDeleteRequest *>(data_);
}

DeleteResponse::DeleteResponse() {
    auto response = new datamovement::DataMovementDeleteResponse();
    data_ = static_cast<void *>(response);
}

DeleteResponse::~DeleteResponse() {
    delete static_cast<datamovement::DataMovementDeleteResponse *>(data_);
}

DeleteResponse::Status DeleteResponse::status() {
    CHECK_ENUM_EQUALITY(DeleteResponse::STATUS_INVALID,   datamovement::DataMovementDeleteResponse_Status_INVALID);
    CHECK_ENUM_EQUALITY(DeleteResponse::STATUS_NOT_FOUND,   datamovement::DataMovementDeleteResponse_Status_NOT_FOUND);
    CHECK_ENUM_EQUALITY(DeleteResponse::STATUS_SUCCESS,   datamovement::DataMovementDeleteResponse_Status_SUCCESS);
    CHECK_ENUM_EQUALITY(DeleteResponse::STATUS_ACTIVE,   datamovement::DataMovementDeleteResponse_Status_ACTIVE);
    CHECK_ENUM_EQUALITY(DeleteResponse::STATUS_UNKNOWN,   datamovement::DataMovementDeleteResponse_Status_UNKNOWN);

    auto status = static_cast<datamovement::DataMovementDeleteResponse *>(data_)->status();
    return static_cast<DeleteResponse::Status>(status);
}

std::string DeleteResponse::message() {
    return static_cast<datamovement::DataMovementDeleteResponse *>(data_)->message();
}

ListRequest::ListRequest() {
    auto request = new datamovement::DataMovementListRequest();
    data_ = static_cast<void *>(request);
}

ListRequest::~ListRequest() {
    delete static_cast<datamovement::DataMovementListRequest *>(data_);
}

ListResponse::ListResponse() {
    auto response = new datamovement::DataMovementListResponse();
    data_ = static_cast<void *>(response);
}

ListResponse::~ListResponse() {
    delete static_cast<datamovement::DataMovementListResponse *>(data_);
}

std::vector<std::string> ListResponse::uids() {
    auto response = static_cast<datamovement::DataMovementListResponse *>(data_);

    auto uids = std::vector<std::string>();
    uids.reserve(response->uids_size());

    for (const std::string& uid : response->uids()) {
        uids.push_back(uid);
    }

    return uids;
}

} // namespace data_movement

} // namespace near_node_flash