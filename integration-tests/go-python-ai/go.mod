module github.com/helix-rpc/helix/integration-tests/go-python-ai

go 1.26.3

require (
	github.com/helix-rpc/helix/runtime-go v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/apache/thrift v0.23.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
)

replace github.com/helix-rpc/helix/runtime-go => ../../runtime-go
