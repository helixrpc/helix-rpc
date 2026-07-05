package runtime

import (
	"fmt"
	"net/http"

	"github.com/apache/thrift/lib/go/thrift"
)

type ErrorCode int

const (
	CodeOK                 ErrorCode = 0
	CodeInvalidArgument    ErrorCode = 3
	CodeNotFound           ErrorCode = 5
	CodeAlreadyExists      ErrorCode = 6
	CodePermissionDenied   ErrorCode = 7
	CodeResourceExhausted  ErrorCode = 8
	CodeUnimplemented      ErrorCode = 12
	CodeInternal           ErrorCode = 13
	CodeUnavailable        ErrorCode = 14
	CodeUnauthenticated    ErrorCode = 16
)

type HelixError struct {
	Code    ErrorCode
	Message string
}

func (e *HelixError) Error() string {
	return fmt.Sprintf("helix error: code=%d message=%s", e.Code, e.Message)
}

func NewError(code ErrorCode, message string) error {
	return &HelixError{Code: code, Message: message}
}

// MapToHTTPStatus maps Helix error codes to standard HTTP/1.1 status codes
func MapToHTTPStatus(code ErrorCode) int {
	switch code {
	case CodeOK:
		return http.StatusOK
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists:
		return http.StatusConflict
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeResourceExhausted:
		return http.StatusTooManyRequests
	case CodeUnimplemented:
		return http.StatusNotImplemented
	case CodeInternal:
		return http.StatusInternalServerError
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// ToThriftException maps standard Go errors into Apache Thrift TApplicationException
func ToThriftException(err error) thrift.TException {
	if err == nil {
		return nil
	}
	if he, ok := err.(*HelixError); ok {
		switch he.Code {
		case CodeUnimplemented:
			return thrift.NewTApplicationException(thrift.UNKNOWN_METHOD, he.Message)
		case CodeInvalidArgument:
			return thrift.NewTApplicationException(thrift.PROTOCOL_ERROR, he.Message)
		default:
			return thrift.NewTApplicationException(thrift.INTERNAL_ERROR, he.Message)
		}
	}
	return thrift.NewTApplicationException(thrift.INTERNAL_ERROR, err.Error())
}

// ToGRPCStatus maps standard Go errors to gRPC status code and message strings
func ToGRPCStatus(err error) (string, string) {
	if err == nil {
		return "0", ""
	}
	if he, ok := err.(*HelixError); ok {
		return fmt.Sprintf("%d", he.Code), he.Message
	}
	return "13", err.Error() // Default to INTERNAL
}
