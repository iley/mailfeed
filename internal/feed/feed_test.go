package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mmcdole/gofeed"
)

func TestParseRSS(t *testing.T) {
	f, err := os.Open("../../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Test Blog", "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// First item: has content:encoded and description, should prefer content:encoded
	if items[0].Title != "First Post" {
		t.Errorf("expected title 'First Post', got %q", items[0].Title)
	}
	if items[0].Link != "https://example.com/first" {
		t.Errorf("expected link 'https://example.com/first', got %q", items[0].Link)
	}
	if items[0].GUID != "https://example.com/first" {
		t.Errorf("expected GUID 'https://example.com/first', got %q", items[0].GUID)
	}
	if items[0].Content != "<p>This is the <b>full content</b> of the first post.</p>" {
		t.Errorf("expected content:encoded content, got %q", items[0].Content)
	}
	if items[0].FeedName != "Test Blog" {
		t.Errorf("expected feed name 'Test Blog', got %q", items[0].FeedName)
	}
	if items[0].FeedURL != "https://example.com/feed.xml" {
		t.Errorf("expected feed URL 'https://example.com/feed.xml', got %q", items[0].FeedURL)
	}
}

func TestContentFallback(t *testing.T) {
	f, err := os.Open("../../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Test Blog", "https://example.com/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	// Second item: has only description, no content:encoded
	if items[1].Content != "Second post has only a description." {
		t.Errorf("expected description as fallback content, got %q", items[1].Content)
	}
}

func TestGUIDFallback(t *testing.T) {
	f, err := os.Open("../../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Test Blog", "https://example.com/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	// Third item: has no guid, should fall back to link
	if items[2].GUID != "https://example.com/third" {
		t.Errorf("expected GUID to fall back to link, got %q", items[2].GUID)
	}
}

func TestGUIDSyntheticWhenEmpty(t *testing.T) {
	item1 := mapItem(&gofeed.Item{Title: "Post A", Description: "content A"}, "Blog", "https://example.com/feed")
	item2 := mapItem(&gofeed.Item{Title: "Post B", Description: "content B"}, "Blog", "https://example.com/feed")
	item3 := mapItem(&gofeed.Item{Title: "Post A", Description: "content A"}, "Blog", "https://example.com/feed")

	if item1.GUID == "" {
		t.Error("expected synthetic GUID, got empty string")
	}
	if !strings.HasPrefix(item1.GUID, "sha256:") {
		t.Errorf("expected sha256: prefix, got %q", item1.GUID)
	}
	if item1.GUID == item2.GUID {
		t.Error("different items should have different synthetic GUIDs")
	}
	if item1.GUID != item3.GUID {
		t.Error("identical items should have the same synthetic GUID")
	}
}

func TestParseAtom(t *testing.T) {
	f, err := os.Open("../../testdata/sample_atom.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Atom Feed", "https://example.com/atom.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].Title != "Atom Entry One" {
		t.Errorf("expected title 'Atom Entry One', got %q", items[0].Title)
	}
	if items[0].Link != "https://example.com/atom-one" {
		t.Errorf("expected link, got %q", items[0].Link)
	}
	if items[0].GUID != "urn:uuid:atom-entry-1" {
		t.Errorf("expected GUID 'urn:uuid:atom-entry-1', got %q", items[0].GUID)
	}
	if items[0].Content != "<p>Full content of atom entry one.</p>" {
		t.Errorf("expected content, got %q", items[0].Content)
	}

	// Second entry: has only summary, should fall back
	if items[1].Content != "Summary of atom entry two." {
		t.Errorf("expected summary as fallback content, got %q", items[1].Content)
	}
}

func TestFetchHTTP(t *testing.T) {
	rssData, err := os.ReadFile("../../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(rssData)
	}))
	defer srv.Close()

	items, err := Fetch(context.Background(), srv.URL, "HTTP Test", srv.URL, "mailfeed/1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].FeedName != "HTTP Test" {
		t.Errorf("expected feed name 'HTTP Test', got %q", items[0].FeedName)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, "Test", srv.URL, "mailfeed/1.0")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestFetchSendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	Fetch(context.Background(), srv.URL, "Test", srv.URL, "custom-agent/2.0")
	if gotUA != "custom-agent/2.0" {
		t.Errorf("expected User-Agent 'custom-agent/2.0', got %q", gotUA)
	}
}

func TestFetchResponseSizeLimit(t *testing.T) {
	// Serve a body larger than maxResponseBytes. The parser should fail
	// because it receives truncated (invalid) XML.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		// Write a valid RSS start, then pad with enough data to exceed the limit.
		fmt.Fprint(w, `<?xml version="1.0"?><rss><channel><title>Big</title><item><title>x</title><description>`)
		buf := make([]byte, 1024)
		for i := range buf {
			buf[i] = 'A'
		}
		for range (maxResponseBytes / 1024) + 1 {
			w.Write(buf)
		}
		fmt.Fprint(w, `</description></item></channel></rss>`)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, "Test", srv.URL, "mailfeed/1.0")
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
}

func TestFetchRetryOn500(t *testing.T) {
	rssData, err := os.ReadFile("../../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write(rssData)
	}))
	defer srv.Close()

	items, err := Fetch(context.Background(), srv.URL, "Test", srv.URL, "mailfeed/1.0")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestFetchNoRetryOn404(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, "Test", srv.URL, "mailfeed/1.0")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("expected 1 attempt for 404, got %d", got)
	}
}

func TestFetchExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, "Test", srv.URL, "mailfeed/1.0")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}
