module github.com/helix-rpc/helix/integration-tests/go-go

go 1.25.0

require (
	github.com/apache/thrift v0.23.0
	github.com/golang/protobuf v1.5.4
	github.com/helix-rpc/helix/runtime-go v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.56.0
	google.golang.org/grpc v1.81.1
)

require (
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/helix-rpc/helix/runtime-go => ../../runtime-go
