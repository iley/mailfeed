package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iley/mailfeed/internal/feed"
)

const (
	blogURL  = "https://example.com/blog/feed.xml"
	freshURL = "https://example.com/fresh/feed.xml"
	feedAURL = "https://example.com/feed-a.xml"
	feedBURL = "https://example.com/feed-b.xml"
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
	s.MarkSeen(blogURL, "guid-1")
	s.MarkSeen(blogURL, "guid-2")

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !loaded.HasSeen(blogURL, "guid-1") {
		t.Error("expected guid-1 to be seen")
	}
	if !loaded.HasSeen(blogURL, "guid-2") {
		t.Error("expected guid-2 to be seen")
	}
	if loaded.HasSeen(blogURL, "guid-3") {
		t.Error("expected guid-3 to not be seen")
	}
}

func TestSaveAtomicNoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.MarkSeen(blogURL, "guid-1")
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
	s.KnownFeeds[blogURL] = true
	s.MarkSeen(blogURL, "old-1")

	items := []feed.Item{
		{FeedName: "Blog", FeedURL: blogURL, GUID: "old-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", FeedURL: blogURL, GUID: "new-1", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", FeedURL: blogURL, GUID: "new-2", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
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
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "a", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "b", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "c", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 item for new feed, got %d", len(result))
	}
	if result[0].GUID != "b" {
		t.Errorf("expected latest item (guid=b), got guid=%s", result[0].GUID)
	}

	// Other items should be marked as seen.
	if !s.HasSeen(blogURL, "a") {
		t.Error("expected older item 'a' to be marked seen")
	}
	if !s.HasSeen(blogURL, "c") {
		t.Error("expected older item 'c' to be marked seen")
	}
	// The latest should NOT be marked yet (caller marks after send).
	if s.HasSeen(blogURL, "b") {
		t.Error("expected latest item 'b' to not be marked seen yet")
	}
}

func TestFilterMixedFeeds(t *testing.T) {
	knownURL := "https://example.com/known/feed.xml"
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds[knownURL] = true
	s.MarkSeen(knownURL, "known-old")

	items := []feed.Item{
		// Known feed: one seen, one new.
		{FeedName: "Known", FeedURL: knownURL, GUID: "known-old", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Known", FeedURL: knownURL, GUID: "known-new", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		// New feed: three items, only latest should be returned.
		{FeedName: "Fresh", FeedURL: freshURL, GUID: "fresh-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Fresh", FeedURL: freshURL, GUID: "fresh-2", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Fresh", FeedURL: freshURL, GUID: "fresh-3", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
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
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds[blogURL] = true
	s.MarkSeen(blogURL, "old-gone-1")
	s.MarkSeen(blogURL, "old-gone-2")

	items := []feed.Item{
		{FeedName: "Blog", FeedURL: blogURL, GUID: "new-1", PublishedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", FeedURL: blogURL, GUID: "new-2", PublishedAt: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Blog", FeedURL: blogURL, GUID: "new-3", PublishedAt: time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)},
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
	s.KnownFeeds[blogURL] = true
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.KnownFeeds[blogURL] {
		t.Error("expected blog URL to be in known feeds after reload")
	}
}

func TestSameGUIDAcrossFeeds(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds[feedAURL] = true
	s.KnownFeeds[feedBURL] = true
	s.MarkSeen(feedAURL, "guid-1")

	items := []feed.Item{
		{FeedName: "Feed A", FeedURL: feedAURL, GUID: "guid-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Feed B", FeedURL: feedBURL, GUID: "guid-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].FeedName != "Feed B" {
		t.Errorf("expected item from Feed B, got %s", result[0].FeedName)
	}
}

func TestPruneOldEntries(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds[blogURL] = true

	// Add an old entry (>90 days ago) and a recent one.
	oldTime := time.Now().Add(-91 * 24 * time.Hour)
	recentTime := time.Now().Add(-1 * time.Hour)
	s.Seen[seenKey(blogURL, "old-item")] = oldTime
	s.Seen[seenKey(blogURL, "recent-item")] = recentTime

	s.Prune()

	if _, ok := s.Seen[seenKey(blogURL, "old-item")]; ok {
		t.Error("expected old entry to be pruned")
	}
	if _, ok := s.Seen[seenKey(blogURL, "recent-item")]; !ok {
		t.Error("expected recent entry to be kept")
	}
	// KnownFeeds should be untouched.
	if !s.KnownFeeds[blogURL] {
		t.Error("expected KnownFeeds to be preserved")
	}
}

func TestSavePrunes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.Seen[seenKey(blogURL, "old")] = time.Now().Add(-91 * 24 * time.Hour)
	s.Seen[seenKey(blogURL, "new")] = time.Now()

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.HasSeen(blogURL, "old") {
		t.Error("expected old entry to be pruned during save")
	}
	if !loaded.HasSeen(blogURL, "new") {
		t.Error("expected new entry to survive save")
	}
}

func TestRenamedFeedStaysKnown(t *testing.T) {
	// Renaming a feed in config should not treat it as new.
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds[blogURL] = true
	s.MarkSeen(blogURL, "post-1")

	items := []feed.Item{
		{FeedName: "Renamed Blog", FeedURL: blogURL, GUID: "post-1", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Renamed Blog", FeedURL: blogURL, GUID: "post-2", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Renamed Blog", FeedURL: blogURL, GUID: "post-3", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items)
	// Should return both new items, not just latest (feed is known).
	if len(result) != 2 {
		t.Fatalf("expected 2 new items after rename, got %d", len(result))
	}
	for _, item := range result {
		if item.GUID == "post-1" {
			t.Error("should not include already-seen item")
		}
	}
}
