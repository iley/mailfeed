package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load("../../testdata/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(cfg.Feeds))
	}
	if cfg.Feeds[0].Name != "Test Blog" {
		t.Errorf("expected feed name 'Test Blog', got %q", cfg.Feeds[0].Name)
	}
	if cfg.Feeds[0].URL != "https://example.com/feed.xml" {
		t.Errorf("expected feed URL 'https://example.com/feed.xml', got %q", cfg.Feeds[0].URL)
	}
	if cfg.Email.From != "mailfeed@example.com" {
		t.Errorf("expected from 'mailfeed@example.com', got %q", cfg.Email.From)
	}
	if cfg.Email.To != "me@example.com" {
		t.Errorf("expected to 'me@example.com', got %q", cfg.Email.To)
	}
	if cfg.Email.SMTP.Host != "smtp.example.com" {
		t.Errorf("expected SMTP host 'smtp.example.com', got %q", cfg.Email.SMTP.Host)
	}
	if cfg.Email.SMTP.Port != 465 {
		t.Errorf("expected SMTP port 465, got %d", cfg.Email.SMTP.Port)
	}
	if cfg.CheckInterval != "30m" {
		t.Errorf("expected check_interval '30m', got %q", cfg.CheckInterval)
	}
}

func TestCheckIntervalDuration(t *testing.T) {
	cfg := Config{CheckInterval: "30m"}
	d, err := cfg.CheckIntervalDuration()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 30*time.Minute {
		t.Errorf("expected 30m, got %v", d)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("{{invalid yaml")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadMissingFeeds(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("email:\n  from: a@b.com\n  to: c@d.com\n")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Fatal("expected error for missing feeds")
	}
}

func TestLoadMissingFeedURL(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("feeds:\n  - name: Test\nemail:\n  from: a@b.com\n  to: c@d.com\n")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Fatal("expected error for missing feed URL")
	}
}
