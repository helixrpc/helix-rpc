package runtime

import (
	"context"
	"sync"
)

// HealthStatus represents the serving status of a service.
type HealthStatus int32

const (
	HealthUnknown    HealthStatus = 0
	HealthServing    HealthStatus = 1
	HealthNotServing HealthStatus = 2
)

// HealthChecker manages per-service health status and registers
// itself as a gRPC method handler.
type HealthChecker struct {
	mu       sync.RWMutex
	statuses map[string]HealthStatus
}

// NewHealthChecker creates a HealthChecker with all services serving.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		statuses: map[string]HealthStatus{
			"": HealthServing, // overall server health
		},
	}
}

// SetServingStatus sets the health status for a service.
func (hc *HealthChecker) SetServingStatus(service string, status HealthStatus) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.statuses[service] = status
}

// Check returns the health status for a service.
func (hc *HealthChecker) Check(service string) HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	if s, ok := hc.statuses[service]; ok {
		return s
	}
	return HealthUnknown
}

// RegisterHealthMethods registers the gRPC health checking service.
// Path: /grpc.health.v1.Health/Check
// Request is a simple JSON/protobuf message with a "service" field.
// Response has a "status" field with the health status.
func RegisterHealthMethods(handler *GRPCHandler, hc *HealthChecker) {
	handler.RegisterMethod("/grpc.health.v1.Health/Check", MethodInfo{
		Decoder: func(dec func(interface{}) error) (interface{}, error) {
			req := &healthCheckRequest{}
			return req, dec(req)
		},
		Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
			hcReq := req.(*healthCheckRequest)
			status := hc.Check(hcReq.Service)
			return &healthCheckResponse{Status: status}, nil
		},
	})
}

type healthCheckRequest struct {
	Service string `json:"service"`
}

type healthCheckResponse struct {
	Status HealthStatus `json:"status"`
}

// Implement ProtoMarshaler and ProtoUnmarshaler for health check messages
// to support both gRPC binary and JSON transports.

func (r *healthCheckRequest) Unmarshal(data []byte) error {
	// Simple protobuf: field 1, type string = tag 0x0a
	i := 0
	for i < len(data) {
		tag := data[i]
		i++
		if tag == 0x0a { // field 1, length-delimited
			if i >= len(data) {
				break
			}
			l := int(data[i])
			i++
			if i+l <= len(data) {
				r.Service = string(data[i : i+l])
			}
			return nil
		}
	}
	return nil
}

func (r *healthCheckResponse) Marshal() ([]byte, error) {
	// Simple protobuf: field 1, type varint = tag 0x08
	if r.Status == 0 {
		return []byte{}, nil
	}
	return []byte{0x08, byte(r.Status)}, nil
}
