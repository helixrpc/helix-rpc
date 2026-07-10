package main

import (
	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/server"
)

type Config struct {
	EnableJWT       bool    `json:"enable_jwt"`
	JWTSecret       string  `json:"jwt_secret"`
	RateLimitRate   float64 `json:"rate_limit_rate"`
	RateLimitBurst  int     `json:"rate_limit_burst"`
	EnableTelemetry bool    `json:"enable_telemetry"`
}

func New() interface{} {
	return &Config{}
}

func (conf *Config) Access(kong *pdk.PDK) {
	// Protocol Sniffing at the Edge
	upgrade, err := kong.Request.GetHeader("upgrade")
	if err == nil && upgrade == "websocket" {
		kong.Response.SetHeader("x-helix-edge-protocol", "websocket")
	}

	contentType, err := kong.Request.GetHeader("content-type")
	if err == nil {
		if contentType == "application/grpc" {
			kong.Response.SetHeader("x-helix-edge-protocol", "grpc")
		} else if contentType == "application/x-flatbuffers" {
			kong.Response.SetHeader("x-helix-edge-protocol", "flatbuffers")
		} else if contentType == "application/json" {
			kong.Response.SetHeader("x-helix-edge-protocol", "json")
		}
	}

	// Example: JWT Edge validation
	if conf.EnableJWT {
		authHeader, err := kong.Request.GetHeader("authorization")
		if err != nil || authHeader == "" {
			kong.Response.Exit(401, []byte("Unauthorized: Missing JWT"), map[string][]string{
				"Content-Type": {"text/plain"},
			})
			return
		}
		// Decode/Verify JWT using conf.JWTSecret (pseudo-code logic)
		// For now we just validate that it's provided if enabled.
	}

	// In a full implementation, Rate Limit counters would be checked against Kong's KV store
	// using conf.RateLimitRate and conf.RateLimitBurst

	if conf.EnableTelemetry {
		kong.Response.SetHeader("x-helix-edge-telemetry", "enabled")
	}
}

func main() {
	server.StartServer(New, "0.1", 1000)
}
