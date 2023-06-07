/*
 * Copyright 2022-2023 Hewlett Packard Enterprise Development LP
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
#include <string>
#include <memory>

#include "../client.h"

using namespace near_node_flash::data_movement;

int main(int argc, char** argv) {
    std::string uid;

    DataMoverClient client("unix:///tmp/nnf.sock");

    {
        // Retrieve the version information

        VersionResponse versionResponse;

        RPCStatus status = client.Version(&versionResponse);
        if (!status.ok()) {
            std::cout << "Version RPC FAILED (" << status.error_code() << "): " << status.error_message() << std::endl;
            return 1;
        }

        std::cout << "Data Movement Version: " << versionResponse.version() << std::endl;
        std::cout << "Supported API Versions:" << std::endl;
        for (auto version : versionResponse.apiversions()) {
            std::cout << "\t" << version << std::endl;
        }

    }

    // Allocate a workflow that will be used by all requests
    Workflow workflow("YOUR-WORKFLOW_NAME", "YOUR-WORKFLOW-NAMESPACE");


    {
        // Create an offload request
        CreateRequest createRequest("YOUR-SOURCE", "YOUR-DESTINATION", false, "", false, false);
        CreateResponse createResponse;

        RPCStatus status = client.Create(workflow, createRequest, &createResponse);
        if (!status.ok()) {
            std::cout << "Create RPC FAILED (" << status.error_code() << "): " << status.error_message() << std::endl;
            return 1;
        }

        switch (createResponse.status()) {
            case CreateResponse::Status::STATUS_SUCCESS:
                uid = createResponse.uid();
                std::cout << "Offload Created: UID: " << createResponse.uid() << std::endl;
                break;
            default:
                std::cout << "Offload Create FAILED: " << createResponse.status() << ": " << createResponse.message() << std::endl;
                return 1;
        }
    }

    {
        ListRequest listRequest;
        ListResponse listResponse;

        RPCStatus status = client.List(workflow, listRequest, &listResponse);
        if (!status.ok()) {
            std::cout << "List RPC FAILED (" << status.error_code() << "): " << status.error_message() << std::endl;
            return 1;
        }

        std::cout << "Offload List: " << listResponse.uids().size() << std::endl;
        for (auto uid : listResponse.uids()) {
            std::cout << "\t" << uid << std::endl;
        }
    }
    
    {
        StatusRequest statusRequest(uid, 0);
        StatusResponse statusResponse;

RequestStatus:
        RPCStatus status = client.Status(workflow, statusRequest, &statusResponse);
        if (!status.ok()) {
            std::cout << "Status RPC FAILED (" << status.error_code() << "): " << status.error_message() << std::endl;
            return 1;
        }

        switch (statusResponse.state()) {
            case StatusResponse::STATE_PENDING:
            case StatusResponse::STATE_STARTING:
            case StatusResponse::STATE_RUNNING:
                std::cout << "Offload State Pending/Starting/Running" << std::endl;
                goto RequestStatus;
            case StatusResponse::STATE_COMPLETED:
                std::cout << "Offload State Completed" << std::endl;
                break;
            default:
                std::cout << "Offload State STATE UNKNOWN: " << statusResponse.state() << " Status: " << statusResponse.status() << std::endl;
                return 1;
        }

        CommandStatus cmd = statusResponse.commandStatus();
        std::cout << "Offload Command Status:" << std::endl;
        std::cout << "  Command: " << cmd.command <<  std::endl;
        std::cout << "  Progress: " << cmd.progress <<  "%" << std::endl;
        std::cout << "  ElapsedTime: " << cmd.elapsedTime <<  std::endl;
        std::cout << "  LastMessage: " << cmd.lastMessage <<  std::endl;
        std::cout << "  LastMessageTime: " << cmd.lastMessageTime <<  std::endl;
        std::cout << "Offload StartTime: " << statusResponse.startTime() << std::endl;
        std::cout << "Offload EndTime: " << statusResponse.endTime() << std::endl;
        std::cout << "Offload Message: " << statusResponse.message() << std::endl;

        switch (statusResponse.status()) {
            case StatusResponse::STATUS_SUCCESS:
                std::cout << "Offload Status Successful" << std::endl;
                break;
            default:
                std::cout << "Offload Status UNSUCCESSFUL: " << statusResponse.status() << " " << statusResponse.message() << std::endl;
                return 1;
        }
    }

    {
        CancelRequest cancelRequest(uid);
        CancelResponse cancelResponse;

        RPCStatus status = client.Cancel(workflow, cancelRequest, &cancelResponse);
        if (!status.ok()) {
            std::cout << "Cancel RPC FAILED (" << status.error_code() << "): " << status.error_message() << std::endl;
            return 1;
        }

        switch (cancelResponse.status()) {
            case CancelResponse::STATUS_SUCCESS:
                std::cout << "Offload Cancel Successful" << std::endl;
                break;
            default:
                std::cout << "Offload Cancel UNSUCCESSFUL: " << cancelResponse.status() << " " << cancelResponse.message() << std::endl;
                return 1;
        }
    }

    {
        DeleteRequest deleteRequest(uid);
        DeleteResponse deleteResponse;

        RPCStatus status = client.Delete(workflow, deleteRequest, &deleteResponse);
        if (!status.ok()) {
            std::cout << "Delete RPC FAILED (" << status.error_code() << "): " << status.error_message() << std::endl;
            return 1;
        }

        switch (deleteResponse.status()) {
            case DeleteResponse::STATUS_SUCCESS:
                std::cout << "Offload Delete Successful" << std::endl;
                break;
            default:
                std::cout << "Offload Delete UNSUCCESSFUL: " << deleteResponse.status() << " " << deleteResponse.message() << std::endl;
                return 1;
        }
    }

    return 0;
}