
all : client.a client

client.a : client.go
	GODEBUG=cgocheck=2 go build -buildmode=c-archive client.go

client : client.a client.c client.h rsyncdatamovement.pb-c.h
	gcc -pthread client.c client.a -o client

# protoc is used to generate the typedefs; no actual C code is used since
# it is not using GRPC (it uses protoc-c-rpc, which is a low-level protocol)
protoc :
	protoc --c_out=. --proto_path=../api ../api/rsyncdatamovement.proto 

clean :
	rm client client.a client.h
