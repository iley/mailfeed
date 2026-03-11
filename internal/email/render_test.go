package email

import (
	"strings"
	"testing"
	"time"

	"github.com/iley/mailfeed/internal/feed"
)

var sampleItem = feed.Item{
	FeedName:    "Test Blog",
	Title:       "Hello World",
	Link:        "https://example.com/hello",
	Content:     "<p>This is <b>bold</b> and <a href=\"https://example.com\">a link</a>.</p>",
	PublishedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
	GUID:        "https://example.com/hello",
}

func TestRenderHTML(t *testing.T) {
	out, err := RenderHTML(sampleItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		name string
		want string
	}{
		{"title link", `<a href="https://example.com/hello"`},
		{"title text", "Hello World"},
		{"feed name", "Test Blog"},
		{"date", "January 15, 2024"},
		{"content bold", "<b>bold</b>"},
		{"content link", `href="https://example.com"`},
		{"view original", "View original"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: expected output to contain %q", c.name, c.want)
		}
	}
}

func TestRenderHTML_SpecialCharsInTitle(t *testing.T) {
	item := sampleItem
	item.Title = "Tom & Jerry <forever>"
	item.Content = "<p>Content with <em>emphasis</em></p>"

	out, err := RenderHTML(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Title should be escaped
	if !strings.Contains(out, "Tom &amp; Jerry &lt;forever&gt;") {
		t.Error("title special chars should be HTML-escaped")
	}
	// Content should NOT be escaped
	if !strings.Contains(out, "<em>emphasis</em>") {
		t.Error("content HTML should not be escaped")
	}
}

func TestRenderHTML_ZeroTime(t *testing.T) {
	item := sampleItem
	item.PublishedAt = time.Time{}

	out, err := RenderHTML(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, "0001") {
		t.Error("zero time should not render year 0001")
	}
}

func TestRenderHTML_EmptyContent(t *testing.T) {
	item := sampleItem
	item.Content = ""

	_, err := RenderHTML(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderPlainText(t *testing.T) {
	out, err := RenderPlainText(sampleItem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Hello World") {
		t.Error("expected title in plain text")
	}
	if !strings.Contains(out, "https://example.com/hello") {
		t.Error("expected link in plain text")
	}
	if !strings.Contains(out, "Test Blog") {
		t.Error("expected feed name in plain text")
	}
	if !strings.Contains(out, "January 15, 2024") {
		t.Error("expected date in plain text")
	}
	if strings.Contains(out, "<p>") || strings.Contains(out, "<b>") {
		t.Error("plain text should not contain HTML tags")
	}
	if !strings.Contains(out, "bold") {
		t.Error("expected stripped text content")
	}
}

func TestRenderHTML_Sanitization(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantAbsent string // must NOT appear in output
		wantIn     string // must appear in output (empty = skip check)
	}{
		{
			name:       "script removed",
			content:    `<p>Hello</p><script>alert("xss")</script>`,
			wantAbsent: "<script",
			wantIn:     "Hello",
		},
		{
			name:       "iframe removed",
			content:    `<p>Text</p><iframe src="https://evil.com"></iframe>`,
			wantAbsent: "<iframe",
			wantIn:     "Text",
		},
		{
			name:       "event handler stripped",
			content:    `<a href="https://example.com" onclick="alert(1)">link</a>`,
			wantAbsent: "onclick",
			wantIn:     "https://example.com",
		},
		{
			name:       "form removed",
			content:    `<form action="/steal"><input type="text"></form>`,
			wantAbsent: "<form",
		},
		{
			name:    "safe tags preserved",
			content: `<p>Text with <b>bold</b> and <em>emphasis</em></p>`,
			wantIn:  "<b>bold</b>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := sampleItem
			item.Content = tt.content
			out, err := RenderHTML(item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantAbsent != "" && strings.Contains(out, tt.wantAbsent) {
				t.Errorf("output should not contain %q", tt.wantAbsent)
			}
			if tt.wantIn != "" && !strings.Contains(out, tt.wantIn) {
				t.Errorf("output should contain %q", tt.wantIn)
			}
		})
	}
}

func TestRenderDigestHTML(t *testing.T) {
	items := []feed.Item{
		{
			FeedName:    "Test Blog",
			Title:       "First Post",
			Link:        "https://example.com/first",
			Content:     "<p>First content</p>",
			PublishedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			FeedName:    "Test Blog",
			Title:       "Second Post",
			Link:        "https://example.com/second",
			Content:     "<p>Second content</p>",
			PublishedAt: time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC),
		},
	}

	out, err := RenderDigestHTML("Test Blog", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"Test Blog",
		"Digest (2 items)",
		"First Post",
		"Second Post",
		"https://example.com/first",
		"https://example.com/second",
		"First content",
		"Second content",
		"mailfeed",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
}

func TestRenderDigestPlainText(t *testing.T) {
	items := []feed.Item{
		{
			FeedName:    "Test Blog",
			Title:       "First Post",
			Link:        "https://example.com/first",
			Content:     "<p>First content</p>",
			PublishedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			FeedName:    "Test Blog",
			Title:       "Second Post",
			Link:        "https://example.com/second",
			Content:     "<p>Second content</p>",
			PublishedAt: time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC),
		},
	}

	out, err := RenderDigestPlainText("Test Blog", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []string{
		"Test Blog",
		"Digest (2 items)",
		"First Post",
		"Second Post",
		"First content",
		"Second content",
		"---",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
	if strings.Contains(out, "<p>") {
		t.Error("plain text should not contain HTML tags")
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic tags",
			input: "<p>Hello <b>world</b></p>",
			want:  "Hello world",
		},
		{
			name:  "link",
			input: `<a href="https://example.com">link text</a>`,
			want:  "link text",
		},
		{
			name:  "plain text passthrough",
			input: "no tags here",
			want:  "no tags here",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "block elements add newlines",
			input: "<p>First</p><p>Second</p>",
			want:  "First\nSecond",
		},
		{
			name:  "self-closing br",
			input: "line one<br/>line two<br />line three",
			want:  "line one\nline two\nline three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
