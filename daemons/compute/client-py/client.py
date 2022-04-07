import grpc
import rsyncdatamovement_pb2
import rsyncdatamovement_pb2_grpc

if __name__ == '__main__':
    with grpc.insecure_channel('unix:///var/run/nnf-dm.sock') as channel:
        stub = rsyncdatamovement_pb2_grpc.RsyncDataMoverStub(channel)

        # Create
        create_request = rsyncdatamovement_pb2.RsyncDataMovementCreateRequest()
        create_response = stub.Create(create_request)
        print(create_response)

        # Status
        status_request = rsyncdatamovement_pb2.RsyncDataMovementStatusRequest(uid=create_response.uid)
        status_response = stub.Status(status_request)
        print(status_response)

        # Delete
        delete_request = rsyncdatamovement_pb2.RsyncDataMovementDeleteRequest(uid=create_response.uid)
        delete_response = stub.Delete(delete_request)
        print(delete_response)