package runtime

import (
	"context"
	"log"
	"net"

	"github.com/apache/thrift/lib/go/thrift"
)

type ThriftProcessor interface {
	Process(ctx context.Context, iprot, oprot thrift.TProtocol) (bool, thrift.TException)
}

func HandleThriftConnection(conn net.Conn, processor ThriftProcessor, isCompact bool) {
	defer conn.Close()

	// Wrap socket in thrift socket transport
	tSocket := thrift.NewTSocketFromConnConf(conn, nil)
	
	// Wrap in framed transport
	transport := thrift.NewTFramedTransport(tSocket)
	defer transport.Close()

	if !transport.IsOpen() {
		if err := transport.Open(); err != nil {
			log.Printf("server thrift transport.Open error: %v", err)
			return
		}
	}

	var protocolFactory thrift.TProtocolFactory
	if isCompact {
		protocolFactory = thrift.NewTCompactProtocolFactoryConf(nil)
	} else {
		protocolFactory = thrift.NewTBinaryProtocolFactoryConf(nil)
	}

	iprot := protocolFactory.GetProtocol(transport)
	oprot := protocolFactory.GetProtocol(transport)

	ctx := context.Background()
	for {
		ok, err := processor.Process(ctx, iprot, oprot)
		if err != nil {
			log.Printf("server thrift process error: %v", err)
			break
		}
		if !ok {
			break
		}
	}
}
