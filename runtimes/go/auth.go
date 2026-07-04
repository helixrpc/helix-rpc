package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ---------------------------------------------------------------------------
// Auth context key
// ---------------------------------------------------------------------------

type authContextKey struct{}

// ClaimsFromContext returns the JWT claims injected by JWTAuthInterceptor.
func ClaimsFromContext(ctx context.Context) (jwt.MapClaims, bool) {
	v, ok := ctx.Value(authContextKey{}).(jwt.MapClaims)
	return v, ok
}

func contextWithClaims(ctx context.Context, claims jwt.MapClaims) context.Context {
	return context.WithValue(ctx, authContextKey{}, claims)
}

// ---------------------------------------------------------------------------
// AuthInterceptor interface — plug in any auth backend
// ---------------------------------------------------------------------------

// AuthInterceptor validates the incoming request and returns an enriched
// context (e.g. with claims/principal). Return a non-nil error to reject.
type AuthInterceptor interface {
	Authenticate(ctx context.Context, headers http.Header) (context.Context, error)
}

// AuthInterceptorFunc adapts a plain function to AuthInterceptor.
type AuthInterceptorFunc func(ctx context.Context, headers http.Header) (context.Context, error)

func (f AuthInterceptorFunc) Authenticate(ctx context.Context, headers http.Header) (context.Context, error) {
	return f(ctx, headers)
}

// ---------------------------------------------------------------------------
// JWTAuthConfig
// ---------------------------------------------------------------------------

// JWTAuthConfig configures the JWT authentication interceptor.
type JWTAuthConfig struct {
	// SecretKey is used for HMAC algorithms (HS256/HS384/HS512).
	// Mutually exclusive with PublicKey.
	SecretKey []byte

	// PublicKeyPEM is the PEM-encoded RSA/EC public key for RS256/ES256.
	// Mutually exclusive with SecretKey.
	PublicKeyPEM []byte

	// Algorithms is the list of accepted signing algorithms.
	// Defaults to ["HS256"] if SecretKey is set, ["RS256"] if PublicKeyPEM is set.
	Algorithms []string

	// RequiredClaims are claim keys that must be present in the token.
	RequiredClaims []string

	// TokenExtractor extracts the raw JWT string from the request.
	// Defaults to extracting from "Authorization: Bearer <token>".
	TokenExtractor func(headers http.Header) (string, error)
}

func defaultBearerExtractor(headers http.Header) (string, error) {
	auth := headers.Get("Authorization")
	if auth == "" {
		// Also check x-api-key style header for convenience
		if key := headers.Get("X-API-Key"); key != "" {
			return key, nil
		}
		return "", &HelixError{Code: CodeUnauthenticated, Message: "missing Authorization header"}
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", &HelixError{Code: CodeUnauthenticated, Message: "malformed Authorization header; expected 'Bearer <token>'"}
	}
	return parts[1], nil
}

// ---------------------------------------------------------------------------
// NewJWTAuthInterceptor
// ---------------------------------------------------------------------------

// NewJWTAuthInterceptor returns a UnaryServerInterceptor that validates
// JWT tokens on every inbound request and injects the parsed claims into ctx.
//
// Example (HMAC):
//
//	interceptor := runtime.NewJWTAuthInterceptor(runtime.JWTAuthConfig{
//	    SecretKey:      []byte("my-secret"),
//	    RequiredClaims: []string{"sub"},
//	})
//	server.AddInterceptor(interceptor)
func NewJWTAuthInterceptor(cfg JWTAuthConfig) UnaryServerInterceptor {
	if cfg.TokenExtractor == nil {
		cfg.TokenExtractor = defaultBearerExtractor
	}
	if len(cfg.Algorithms) == 0 {
		if len(cfg.PublicKeyPEM) > 0 {
			cfg.Algorithms = []string{"RS256", "ES256"}
		} else {
			cfg.Algorithms = []string{"HS256"}
		}
	}

	keyFunc := buildKeyFunc(cfg)

	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		md, _ := FromContext(ctx)
		// Build a synthetic http.Header from the gRPC metadata map
		headers := make(http.Header)
		for k, vs := range md {
			for _, v := range vs {
				headers.Add(k, v)
			}
		}

		rawToken, err := cfg.TokenExtractor(headers)
		if err != nil {
			return nil, err
		}

		token, err := jwt.Parse(rawToken, keyFunc,
			jwt.WithValidMethods(cfg.Algorithms),
			jwt.WithExpirationRequired(),
		)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return nil, &HelixError{Code: CodeUnauthenticated, Message: "token expired"}
			}
			return nil, &HelixError{Code: CodeUnauthenticated, Message: fmt.Sprintf("invalid token: %v", err)}
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			return nil, &HelixError{Code: CodeUnauthenticated, Message: "invalid token claims"}
		}

		// Verify required claims are present
		for _, required := range cfg.RequiredClaims {
			if _, exists := claims[required]; !exists {
				return nil, &HelixError{
					Code:    CodePermissionDenied,
					Message: fmt.Sprintf("missing required claim %q", required),
				}
			}
		}

		ctx = contextWithClaims(ctx, claims)
		return handler(ctx, req)
	}
}

// NewJWTMiddleware returns an http.Handler middleware version for REST-only
// servers that don't use the interceptor chain.
func NewJWTMiddleware(cfg JWTAuthConfig, next http.Handler) http.Handler {
	interceptor := NewJWTAuthInterceptor(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		_, err := interceptor(ctx, nil, &UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
			next.ServeHTTP(w, r.WithContext(ctx))
			return nil, nil
		})
		if err != nil {
			var he *HelixError
			if errors.As(err, &he) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(MapToHTTPStatus(he.Code))
				fmt.Fprintf(w, `{"error":%q,"code":%d}`, he.Message, he.Code)
				return
			}
			http.Error(w, err.Error(), http.StatusUnauthorized)
		}
	})
}

// ---------------------------------------------------------------------------
// APIKeyAuthInterceptor — simple static API key validation
// ---------------------------------------------------------------------------

// NewAPIKeyInterceptor validates requests against a static set of API keys.
// Keys are read from the "X-API-Key" or "Authorization: Bearer" header.
func NewAPIKeyInterceptor(validKeys map[string]string) UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		md, _ := FromContext(ctx)
		key := ""
		for _, h := range []string{"x-api-key", "authorization"} {
			if vals, ok := md[h]; ok && len(vals) > 0 {
				key = strings.TrimPrefix(vals[0], "Bearer ")
				break
			}
		}
		if key == "" {
			return nil, &HelixError{Code: CodeUnauthenticated, Message: "missing API key"}
		}
		if _, valid := validKeys[key]; !valid {
			return nil, &HelixError{Code: CodeUnauthenticated, Message: "invalid API key"}
		}
		return handler(ctx, req)
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func buildKeyFunc(cfg JWTAuthConfig) jwt.Keyfunc {
	if len(cfg.PublicKeyPEM) > 0 {
		return func(token *jwt.Token) (interface{}, error) {
			switch token.Method.(type) {
			case *jwt.SigningMethodRSA:
				return jwt.ParseRSAPublicKeyFromPEM(cfg.PublicKeyPEM)
			case *jwt.SigningMethodECDSA:
				return jwt.ParseECPublicKeyFromPEM(cfg.PublicKeyPEM)
			default:
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
		}
	}
	secret := cfg.SecretKey
	return func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	}
}

// Ensure the jwt dep is available at compile time
var _ = time.Second
