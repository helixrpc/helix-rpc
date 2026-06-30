module github.com/helix-rpc/helix/integration-tests/go-go

go 1.25.0

require (
	github.com/apache/thrift v0.23.0
	github.com/golang/protobuf v1.5.4
)

require (
	github.com/helix-rpc/helix/runtime-go v0.0.0-00010101000000-000000000000 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)

replace github.com/helix-rpc/helix/runtime-go => ../../runtime-go
