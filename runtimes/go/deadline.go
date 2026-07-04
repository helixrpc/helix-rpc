package runtime

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// parseGRPCTimeout parses the grpc-timeout header value.
// Format: <value><unit> where unit is n(nano), u(micro), m(milli), S(sec), M(min), H(hour)
func parseGRPCTimeout(val string) (time.Duration, bool) {
	if len(val) < 2 {
		return 0, false
	}
	unit := val[len(val)-1]
	numStr := val[:len(val)-1]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case 'n':
		return time.Duration(num) * time.Nanosecond, true
	case 'u':
		return time.Duration(num) * time.Microsecond, true
	case 'm':
		return time.Duration(num) * time.Millisecond, true
	case 'S':
		return time.Duration(num) * time.Second, true
	case 'M':
		return time.Duration(num) * time.Minute, true
	case 'H':
		return time.Duration(num) * time.Hour, true
	default:
		return 0, false
	}
}

// contextWithDeadlineFromHeaders extracts the grpc-timeout header and applies
// a deadline to the context. Returns the context and an optional cancel function.
func contextWithDeadlineFromHeaders(ctx context.Context, headers map[string][]string) (context.Context, context.CancelFunc) {
	for k, vals := range headers {
		if strings.ToLower(k) == "grpc-timeout" && len(vals) > 0 {
			if d, ok := parseGRPCTimeout(vals[0]); ok {
				return context.WithTimeout(ctx, d)
			}
		}
	}
	return ctx, func() {}
}
