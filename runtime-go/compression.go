package runtime

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

// Compressor handles compression and decompression of gRPC message payloads.
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
	Name() string
}

// GzipCompressor implements gzip compression.
type GzipCompressor struct{}

func (g *GzipCompressor) Name() string { return "gzip" }

func (g *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("gzip compress: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

func (g *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	return out, nil
}

// compressorRegistry maps encoding names to compressor implementations.
var compressorRegistry = map[string]Compressor{
	"gzip": &GzipCompressor{},
}

// RegisterCompressor registers a custom compressor.
func RegisterCompressor(c Compressor) {
	compressorRegistry[c.Name()] = c
}

// getCompressor returns the compressor for the given encoding name.
func getCompressor(name string) Compressor {
	name = strings.TrimSpace(strings.ToLower(name))
	return compressorRegistry[name]
}
