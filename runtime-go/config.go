package runtime

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// Config defines all Helix RPC service configurables.
type Config struct {
	Host             string  `json:"host"`
	Port             int     `json:"port"`
	DisableMetrics   bool    `json:"disable_metrics"`
	DisableHealth    bool    `json:"disable_health"`
	DisableGzip      bool    `json:"disable_gzip"`
	DisableDeadline  bool    `json:"disable_deadline"`
	RateLimitRate    float64 `json:"rate_limit_rate"`
	RateLimitBurst   int     `json:"rate_limit_burst"`
	EnableJWTAuth    bool    `json:"enable_jwt_auth"`
	JWTSecret        string  `json:"jwt_secret"`
	EnableApiKeyAuth bool    `json:"enable_api_key_auth"`
	ApiKey           string  `json:"api_key"`
}

// LoadConfig reads the JSON configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// WatchConfig polls the configuration file for changes and triggers reloading.
func WatchConfig(path string, onChange func(*Config)) {
	go func() {
		var lastMod time.Time
		if info, err := os.Stat(path); err == nil {
			lastMod = info.ModTime()
		}
		for {
			time.Sleep(2 * time.Second)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				if cfg, err := LoadConfig(path); err == nil {
					log.Printf("🧬 [Helix] Dynamic config reload from %s succeeded.", path)
					onChange(cfg)
				} else {
					log.Printf("✗ [Helix] Failed to reload config from %s: %v", path, err)
				}
			}
		}
	}()
}
