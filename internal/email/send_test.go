package email

import (
	"strings"
	"testing"
	"time"

	"github.com/iley/mailfeed/internal/config"
	"github.com/iley/mailfeed/internal/feed"
)

var testItem = feed.Item{
	FeedName:    "Test Blog",
	Title:       "Hello World",
	Link:        "https://example.com/hello",
	Content:     "<p>Some content</p>",
	PublishedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
	GUID:        "hello-123",
}

func TestBuildMessage(t *testing.T) {
	msg, err := buildMessage("from@example.com", "to@example.com", testItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{"from header", "From: from@example.com"},
		{"to header", "To: to@example.com"},
		{"subject with feed name", "Subject: [Test Blog] Hello World"},
		{"mime version", "MIME-Version: 1.0"},
		{"multipart boundary", "Content-Type: multipart/alternative"},
		{"text part", "Content-Type: text/plain; charset=utf-8"},
		{"html part", "Content-Type: text/html; charset=utf-8"},
		{"message-id", "Message-ID: <"},
		{"date header", "Date: "},
		{"html content", "<p>Some content</p>"},
		{"text content", "Some content"},
		{"item link", "https://example.com/hello"},
	}

	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("%s: expected message to contain %q", c.name, c.contains)
		}
	}
}

func TestBuildMessageNoFeedName(t *testing.T) {
	item := testItem
	item.FeedName = ""

	msg, err := buildMessage("from@example.com", "to@example.com", item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(msg, "Test_Blog") || strings.Contains(msg, "Test+Blog") {
		t.Error("expected no feed name in subject when feed name is empty")
	}
	if !strings.Contains(msg, "Hello") {
		t.Error("expected subject to contain title")
	}
}

func TestBuildMessageUnicodeSubject(t *testing.T) {
	item := testItem
	item.Title = "Привет мир"

	msg, err := buildMessage("from@example.com", "to@example.com", item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(msg, "Subject: =?utf-8?") {
		t.Error("expected Q-encoded subject for unicode")
	}
}

func TestHostFromAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"nope", "localhost"},
	}
	for _, tt := range tests {
		got := hostFromAddr(tt.input)
		if got != tt.want {
			t.Errorf("hostFromAddr(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUseImplicitTLS(t *testing.T) {
	tests := []struct {
		port int
		tls  string
		want bool
	}{
		{465, "", true},
		{587, "", false},
		{465, "starttls", false},
		{587, "implicit", true},
		{25, "", false},
	}
	for _, tt := range tests {
		s := NewSender(config.Email{
			SMTP: config.SMTPConfig{Port: tt.port, TLS: tt.tls},
		})
		got := s.useImplicitTLS()
		if got != tt.want {
			t.Errorf("port=%d tls=%q: got %v, want %v", tt.port, tt.tls, got, tt.want)
		}
	}
}

func TestSendAllEmpty(t *testing.T) {
	s := NewSender(config.Email{})
	if err := s.SendAll(nil); err != nil {
		t.Errorf("expected no error for empty items, got %v", err)
	}
}
