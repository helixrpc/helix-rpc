module github.com/helixrpc/examples/go-dynamic-batcher

go 1.25.0

require github.com/helixrpc/helix-rt v0.0.0

require (
	github.com/apache/thrift v0.23.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

replace github.com/helixrpc/helix-rt => ../../runtime-go
