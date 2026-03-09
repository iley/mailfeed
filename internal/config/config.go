package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Feeds         []Feed `yaml:"feeds"`
	Email         Email  `yaml:"email"`
	CheckInterval string `yaml:"check_interval"`
}

type Feed struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type Email struct {
	From string     `yaml:"from"`
	To   string     `yaml:"to"`
	SMTP SMTPConfig `yaml:"smtp"`
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	TLS      string `yaml:"tls"` // "implicit", "starttls", or "" (auto based on port)
}

func (c Config) CheckIntervalDuration() (time.Duration, error) {
	return time.ParseDuration(c.CheckInterval)
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c Config) validate() error {
	if len(c.Feeds) == 0 {
		return fmt.Errorf("no feeds configured")
	}
	for i, f := range c.Feeds {
		if f.URL == "" {
			return fmt.Errorf("feed %d: missing url", i)
		}
	}
	if c.Email.From == "" {
		return fmt.Errorf("missing email.from")
	}
	if c.Email.To == "" {
		return fmt.Errorf("missing email.to")
	}
	switch c.Email.SMTP.TLS {
	case "", "implicit", "starttls":
	default:
		return fmt.Errorf("invalid smtp.tls: %q (must be \"implicit\", \"starttls\", or empty)", c.Email.SMTP.TLS)
	}
	if c.CheckInterval != "" {
		if _, err := time.ParseDuration(c.CheckInterval); err != nil {
			return fmt.Errorf("invalid check_interval: %w", err)
		}
	}
	return nil
}
