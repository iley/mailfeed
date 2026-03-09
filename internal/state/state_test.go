package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iley/mailfeed/internal/feed"
)

func TestLoadNonexistent(t *testing.T) {
	s, err := Load("/nonexistent/state.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Seen) != 0 {
		t.Errorf("expected empty state, got %d entries", len(s.Seen))
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.MarkSeen("guid-1")
	s.MarkSeen("guid-2")

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !loaded.HasSeen("guid-1") {
		t.Error("expected guid-1 to be seen")
	}
	if !loaded.HasSeen("guid-2") {
		t.Error("expected guid-2 to be seen")
	}
	if loaded.HasSeen("guid-3") {
		t.Error("expected guid-3 to not be seen")
	}
}

func TestSaveAtomicNoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.MarkSeen("guid-1")
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify no temp file left behind.
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("expected temp file to be cleaned up")
	}
}

func TestFilterKnownFeed(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds["Blog"] = true
	s.MarkSeen("old-1")

	items := []feed.Item{
		{FeedName: "Blog", GUID: "old-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", GUID: "new-1", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", GUID: "new-2", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	for _, item := range result {
		if item.GUID == "old-1" {
			t.Error("should not include already-seen item")
		}
	}
}

func TestFilterNewFeed(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}

	items := []feed.Item{
		{FeedName: "New Blog", GUID: "a", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "New Blog", GUID: "b", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{FeedName: "New Blog", GUID: "c", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 item for new feed, got %d", len(result))
	}
	if result[0].GUID != "b" {
		t.Errorf("expected latest item (guid=b), got guid=%s", result[0].GUID)
	}

	// Other items should be marked as seen.
	if !s.HasSeen("a") {
		t.Error("expected older item 'a' to be marked seen")
	}
	if !s.HasSeen("c") {
		t.Error("expected older item 'c' to be marked seen")
	}
	// The latest should NOT be marked yet (caller marks after send).
	if s.HasSeen("b") {
		t.Error("expected latest item 'b' to not be marked seen yet")
	}
}

func TestFilterMixedFeeds(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds["Known"] = true
	s.MarkSeen("known-old")

	items := []feed.Item{
		// Known feed: one seen, one new.
		{FeedName: "Known", GUID: "known-old", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Known", GUID: "known-new", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		// New feed: three items, only latest should be returned.
		{FeedName: "Fresh", GUID: "fresh-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Fresh", GUID: "fresh-2", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Fresh", GUID: "fresh-3", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)

	guids := make(map[string]bool)
	for _, item := range result {
		guids[item.GUID] = true
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d: %v", len(result), guids)
	}
	if !guids["known-new"] {
		t.Error("expected known-new in results")
	}
	if !guids["fresh-2"] {
		t.Error("expected fresh-2 (latest) in results")
	}
}

func TestFilterRotatedFeed(t *testing.T) {
	// A known feed whose old items have all rotated out.
	// All current items are unseen, but the feed is known,
	// so all new items should be sent (not just the latest).
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds["Blog"] = true
	s.MarkSeen("old-gone-1")
	s.MarkSeen("old-gone-2")

	items := []feed.Item{
		{FeedName: "Blog", GUID: "new-1", PublishedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", GUID: "new-2", PublishedAt: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", GUID: "new-3", PublishedAt: time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)
	if len(result) != 3 {
		t.Fatalf("expected 3 items for rotated known feed, got %d", len(result))
	}
}

func TestKnownFeedsPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds["Blog"] = true
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.KnownFeeds["Blog"] {
		t.Error("expected Blog to be in known feeds after reload")
	}
}
