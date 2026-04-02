package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	IPMI    IPMIConfig
	TrueNAS TrueNASConfig
}

type IPMIConfig struct {
	Host      string
	User      string
	Password  string
	Privilege string // ipmitool -L: ADMINISTRATOR, OPERATOR, USER
	Interface string // ipmitool -I: lan (IPMI v1.5, default) or lanplus (IPMI v2.0)
	AuthType  string // ipmitool -A: NONE, PASSWORD, MD2, MD5 — forced to skip negotiation
}

type TrueNASConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	KeyFile  string
}

// Load reads configuration from environment variables.
//
// Required:
//
//	IPMI_HOST, IPMI_USER, IPMI_PASSWORD
//	TRUENAS_HOST, TRUENAS_USER, and one of TRUENAS_PASSWORD / TRUENAS_KEY_FILE
//
// Optional (defaults shown):
//
//	IPMI_PRIVILEGE=ADMINISTRATOR, TRUENAS_PORT=22
func Load() (*Config, error) {
	godotenv.Load() // no-op if .env is absent; real env vars take precedence
	cfg := &Config{
		IPMI: IPMIConfig{
			Host:      os.Getenv("IPMI_HOST"),
			User:      os.Getenv("IPMI_USER"),
			Password:  os.Getenv("IPMI_PASSWORD"),
			Privilege: envString("IPMI_PRIVILEGE", "ADMINISTRATOR"),
			Interface: envString("IPMI_INTERFACE", "lan"),
			AuthType:  envString("IPMI_AUTH_TYPE", "PASSWORD"),
		},
		TrueNAS: TrueNASConfig{
			Host:     os.Getenv("TRUENAS_HOST"),
			Port:     envInt("TRUENAS_PORT", 22),
			User:     os.Getenv("TRUENAS_USER"),
			Password: os.Getenv("TRUENAS_PASSWORD"),
			KeyFile:  os.Getenv("TRUENAS_KEY_FILE"),
		},
	}
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.IPMI.Host == "" {
		return fmt.Errorf("IPMI_HOST is required")
	}
	if c.IPMI.User == "" {
		return fmt.Errorf("IPMI_USER is required")
	}
	if c.IPMI.Password == "" {
		return fmt.Errorf("IPMI_PASSWORD is required")
	}
	if c.TrueNAS.Host == "" {
		return fmt.Errorf("TRUENAS_HOST is required")
	}
	if c.TrueNAS.User == "" {
		return fmt.Errorf("TRUENAS_USER is required")
	}
	if c.TrueNAS.Password == "" && c.TrueNAS.KeyFile == "" {
		return fmt.Errorf("TRUENAS_PASSWORD or TRUENAS_KEY_FILE is required")
	}
	return nil
}

func envString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
