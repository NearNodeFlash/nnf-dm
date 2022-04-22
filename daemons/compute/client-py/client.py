# Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
# Other additional copyright holders may be indicated within.
#
# The entirety of this work is licensed under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
#
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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