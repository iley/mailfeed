package feed

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestParseRSS(t *testing.T) {
	f, err := os.Open("../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Test Blog")
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
}

func TestContentFallback(t *testing.T) {
	f, err := os.Open("../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Test Blog")
	if err != nil {
		t.Fatal(err)
	}

	// Second item: has only description, no content:encoded
	if items[1].Content != "Second post has only a description." {
		t.Errorf("expected description as fallback content, got %q", items[1].Content)
	}
}

func TestGUIDFallback(t *testing.T) {
	f, err := os.Open("../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Test Blog")
	if err != nil {
		t.Fatal(err)
	}

	// Third item: has no guid, should fall back to link
	if items[2].GUID != "https://example.com/third" {
		t.Errorf("expected GUID to fall back to link, got %q", items[2].GUID)
	}
}

func TestParseAtom(t *testing.T) {
	f, err := os.Open("../testdata/sample_atom.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	items, err := Parse(f, "Atom Feed")
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
	rssData, err := os.ReadFile("../testdata/sample_rss.xml")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(rssData)
	}))
	defer srv.Close()

	items, err := Fetch(srv.URL, "HTTP Test")
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

	_, err := Fetch(srv.URL, "Test")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
