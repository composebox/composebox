package config

import (
	"flag"
	"os"
)

// Config represents runtime configuration for composebox.
type Config struct {
	Host           string
	Port           string
	StacksDir      string
	Password       string
	NoAuth         bool
	ShowVersion    bool
	TrustedProxies string
}

// Load loads configuration from command line flags and environment variables.
func Load() *Config {
	cfg := &Config{}

	defaultHost := getEnv("HOST", "127.0.0.1")
	defaultPort := getEnv("PORT", "8080")
	defaultStacksDir := getEnv("STACKS_DIR", "./stacks")
	defaultPassword := getEnv("COMPOSEBOX_PASSWORD", "")
	defaultNoAuth := getEnv("NO_AUTH", "false") == "true"
	defaultTrustedProxies := getEnv("TRUSTED_PROXIES", "127.0.0.1,::1")

	flag.StringVar(&cfg.Host, "host", defaultHost, "Host address to bind to (e.g. 127.0.0.1)")
	flag.StringVar(&cfg.Port, "port", defaultPort, "Port to listen on")
	flag.StringVar(&cfg.StacksDir, "stacks", defaultStacksDir, "Directory containing compose stacks")
	flag.StringVar(&cfg.Password, "password", defaultPassword, "Predefined admin password")
	flag.BoolVar(&cfg.NoAuth, "no-auth", defaultNoAuth, "Disable authentication completely")
	flag.StringVar(&cfg.TrustedProxies, "trusted-proxies", defaultTrustedProxies, "Comma-separated list of trusted proxy IPs/CIDRs allowed to set client IP headers")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "Print version information and exit")
	flag.BoolVar(&cfg.ShowVersion, "v", false, "Print version information and exit (shorthand)")

	flag.Parse()

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
