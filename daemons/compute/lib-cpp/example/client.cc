
#include <iostream>
#include <string>
#include <memory>

#include "../client.h"

int main(int argc, char** argv) {
    std::string uid;

    DataMoverClient client("unix:///tmp/nnf.sock");

    // Allocate a workflow that will be used by all requests
    Workflow workflow("YOUR-WORKFLOW_NAME", "YOUR-WORKFLOW-NAMESPACE");


    {
        // Create an offload request
        CreateRequest createRequest("YOUR-SOURCE", "YOUR-DESTINATION");
        CreateResponse createResponse;

        Status status = client.create(workflow, createRequest, &createResponse);
        if (!status.ok()) {
            std::cout << "Create RPC FAILED" << status.error_code() << ": " << status.error_message() << std::endl;
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

        Status status = client.list(workflow, listRequest, &listResponse);
        if (!status.ok()) {
            std::cout << "List RPC FAILED" << status.error_code() << ": " << status.error_message() << std::endl;
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
        Status status = client.status(workflow, statusRequest, &statusResponse);
        if (!status.ok()) {
            std::cout << "Status RPC FAILED" << status.error_code() << ": " << status.error_message() << std::endl;
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

        Status status = client.cancel(workflow, cancelRequest, &cancelResponse);
        if (!status.ok()) {
            std::cout << "Cancle RPC FAILED" << status.error_code() << ": " << status.error_message() << std::endl;
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

        Status status = client.delete_(workflow, deleteRequest, &deleteResponse);
        if (!status.ok()) {
            std::cout << "Delete RPC FAILED" << status.error_code() << ": " << status.error_message() << std::endl;
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