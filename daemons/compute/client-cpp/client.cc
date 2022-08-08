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

#include <iostream>
#include <memory>
#include <string>

#include <grpcpp/grpcpp.h>

#include "datamovement.grpc.pb.h"

using grpc::Channel;
using grpc::ClientContext;
using grpc::Status;
using datamovement::DataMover;
using datamovement::DataMovementCreateRequest;
using datamovement::DataMovementCreateResponse;
using datamovement::DataMovementCreateResponse_Status;
using datamovement::DataMovementStatusRequest;
using datamovement::DataMovementStatusResponse;
using datamovement::DataMovementStatusResponse_Status;
using datamovement::DataMovementDeleteRequest;
using datamovement::DataMovementDeleteResponse;
using datamovement::DataMovementDeleteResponse_Status;


class DataMoverClient {
    public:
        DataMoverClient(std::shared_ptr<Channel> channel) : stub_(DataMover::NewStub(channel)) {}

        std::unique_ptr<DataMover::Stub> stub_;
};

int main(int argc, char** argv) {

    DataMoverClient client(
        grpc::CreateChannel("unix:///tmp/nnf.sock", grpc::InsecureChannelCredentials()));

    std::string uid;
    
    {
        ClientContext context;

        DataMovementCreateRequest createRequest;
        createRequest.set_workflow("YOUR-WORKFLOW");
        createRequest.set_namespace_("YOUR-WORKFLOW-NAMESPACE");
        createRequest.set_source("YOUR-SOURCE");
        createRequest.set_destination("YOUR_DESTINATION");

        DataMovementCreateResponse createResponse;

        Status status = client.stub_->Create(&context, createRequest, &createResponse);
        if (!status.ok()) {
            std::cout << "Create RPC FAILED: " << status.error_code() << ": " << status.error_message() << std::endl;
            return 1;
        }

        switch (createResponse.status()) {
            case datamovement::DataMovementCreateResponse_Status_CREATED:
                std::cout << "Create Success: UID: " << createResponse.uid() << std::endl;
                break;
            default:
                std::cout << "Create Not Successful: " << createResponse.status() << ": " << createResponse.message() << std::endl;
                return 1;
        }

        uid = createResponse.uid();
    }
    
    {
        ClientContext context;

        DataMovementStatusRequest statusRequest;
        statusRequest.set_uid(uid);

        DataMovementStatusResponse statusResponse;

        Status status = client.stub_->Status(&context, statusRequest, &statusResponse);
        if (!status.ok()) {
            std::cout << "Status RPC FAILED: " << status.error_code() << ": " << status.error_message() << std::endl;
            return 1;
        }

        switch (statusResponse.status()) {
            case datamovement::DataMovementStatusResponse_Status_SUCCESS:
                std::cout << "Status Success" << std::endl;
                break;
            default:
                std::cout << "Status Not Successful: " << statusResponse.status() << ": " << statusResponse.message() << std::endl;
                return 1;
        }
    }

    {
        ClientContext context;

        DataMovementDeleteRequest deleteRequest;
        deleteRequest.set_uid(uid);

        DataMovementDeleteResponse deleteResponse;

        Status status = client.stub_->Delete(&context, deleteRequest, &deleteResponse);
        if (!status.ok()) {
            std::cout << "Delete RPC FAILED: " << status.error_code() << ": " << status.error_message() << std::endl;
            return 1;
        }

        switch (deleteResponse.status()) {
            case datamovement::DataMovementDeleteResponse_Status_DELETED:
                std::cout << "Delete Success" << std::endl;
                break;
            default:
                std::cout << "Delete Not Successful: " << deleteResponse.status() << std::endl;
                return 1;
        }
    }


    return 0;
}