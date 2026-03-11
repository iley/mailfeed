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

	result := s.FilterNewItems(items, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	for _, item := range result {
		if item.GUID == "old-1" {
			t.Error("should not include already-seen item")
		}
	}
}

func TestFilterNewFeedRecentItem(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}

	now := time.Now()
	items := []feed.Item{
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "a", PublishedAt: now.Add(-6 * 24 * time.Hour)},
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "b", PublishedAt: now.Add(-1 * 24 * time.Hour)},
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "c", PublishedAt: now.Add(-3 * 24 * time.Hour)},
	}

	result := s.FilterNewItems(items, nil)
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

func TestFilterNewFeedOldItem(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}

	// All items are older than 7 days — none should be sent.
	items := []feed.Item{
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "a", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "b", PublishedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{FeedName: "New Blog", FeedURL: blogURL, GUID: "c", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	result := s.FilterNewItems(items, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 items for new feed with old posts, got %d", len(result))
	}

	// All items should be marked as seen.
	if !s.HasSeen(blogURL, "a") {
		t.Error("expected item 'a' to be marked seen")
	}
	if !s.HasSeen(blogURL, "b") {
		t.Error("expected item 'b' to be marked seen")
	}
	if !s.HasSeen(blogURL, "c") {
		t.Error("expected item 'c' to be marked seen")
	}
}

func TestFilterMixedFeeds(t *testing.T) {
	knownURL := "https://example.com/known/feed.xml"
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.KnownFeeds[knownURL] = true
	s.MarkSeen(knownURL, "known-old")

	now := time.Now()
	items := []feed.Item{
		// Known feed: one seen, one new.
		{FeedName: "Known", FeedURL: knownURL, GUID: "known-old", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{FeedName: "Known", FeedURL: knownURL, GUID: "known-new", PublishedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		// New feed: three items, only latest should be returned (and only if recent).
		{FeedName: "Fresh", FeedURL: freshURL, GUID: "fresh-1", PublishedAt: now.Add(-5 * 24 * time.Hour)},
		{FeedName: "Fresh", FeedURL: freshURL, GUID: "fresh-2", PublishedAt: now.Add(-1 * 24 * time.Hour)},
		{FeedName: "Fresh", FeedURL: freshURL, GUID: "fresh-3", PublishedAt: now.Add(-3 * 24 * time.Hour)},
	}

	result := s.FilterNewItems(items, nil)

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

	result := s.FilterNewItems(items, nil)
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

	result := s.FilterNewItems(items, nil)
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

func TestRecordSend(t *testing.T) {
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}

	if s.SendsToday() != 0 {
		t.Fatalf("expected 0 sends today, got %d", s.SendsToday())
	}

	s.RecordSend()
	s.RecordSend()
	s.RecordSend()

	if s.SendsToday() != 3 {
		t.Fatalf("expected 3 sends today, got %d", s.SendsToday())
	}
}

func TestSendsTodayResetsOnNewDay(t *testing.T) {
	s := &State{
		Seen:       make(map[string]time.Time),
		KnownFeeds: make(map[string]bool),
		DailySends: &DailySends{Date: "1999-01-01", Count: 42},
	}

	// Old date should be treated as zero.
	if s.SendsToday() != 0 {
		t.Fatalf("expected 0 sends today for stale date, got %d", s.SendsToday())
	}

	// Recording a send should reset to today.
	s.RecordSend()
	if s.SendsToday() != 1 {
		t.Fatalf("expected 1 send after RecordSend, got %d", s.SendsToday())
	}
}

func TestDailySendsSurviveSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}
	s.RecordSend()
	s.RecordSend()

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.SendsToday() != 2 {
		t.Errorf("expected 2 sends today after reload, got %d", loaded.SendsToday())
	}
}

func TestLoadStateWithoutDailySends(t *testing.T) {
	// Backward compat: state files from before this feature have no daily_sends field.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{"seen":{},"known_feeds":{}}`), 0o644)

	s, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.SendsToday() != 0 {
		t.Errorf("expected 0 sends today for legacy state, got %d", s.SendsToday())
	}
}

func TestAppendDigestItems(t *testing.T) {
	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}

	sendAt := time.Date(2024, 6, 15, 8, 0, 0, 0, time.UTC)
	items1 := []feed.Item{
		{FeedURL: blogURL, GUID: "a", Title: "Post A"},
		{FeedURL: blogURL, GUID: "b", Title: "Post B"},
	}
	s.AppendDigestItems(blogURL, items1, sendAt)

	if pd, ok := s.PendingDigests[blogURL]; !ok {
		t.Fatal("expected pending digest for blog")
	} else {
		if len(pd.Items) != 2 {
			t.Errorf("expected 2 items, got %d", len(pd.Items))
		}
		if !pd.SendAt.Equal(sendAt) {
			t.Errorf("expected sendAt %v, got %v", sendAt, pd.SendAt)
		}
	}

	// Appending more items should keep the existing sendAt.
	items2 := []feed.Item{
		{FeedURL: blogURL, GUID: "c", Title: "Post C"},
	}
	laterSendAt := sendAt.Add(24 * time.Hour)
	s.AppendDigestItems(blogURL, items2, laterSendAt)

	pd := s.PendingDigests[blogURL]
	if len(pd.Items) != 3 {
		t.Errorf("expected 3 items after append, got %d", len(pd.Items))
	}
	if !pd.SendAt.Equal(sendAt) {
		t.Error("expected original sendAt to be preserved")
	}
}

func TestReadyDigests(t *testing.T) {
	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}

	now := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
	s.PendingDigests[blogURL] = &PendingDigest{
		SendAt: now.Add(-1 * time.Hour), // ready
		Items:  []feed.Item{{GUID: "a"}},
	}
	s.PendingDigests[freshURL] = &PendingDigest{
		SendAt: now.Add(1 * time.Hour), // not ready
		Items:  []feed.Item{{GUID: "b"}},
	}

	ready := s.ReadyDigests(now)
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready digest, got %d", len(ready))
	}
	if _, ok := ready[blogURL]; !ok {
		t.Error("expected blog digest to be ready")
	}
}

func TestDigestSentTodayNewItemsWaitForTomorrow(t *testing.T) {
	// Scenario: digest was sent at 08:00, then at 10:00 new items are fetched.
	// The new items should NOT be immediately ready — they should wait for
	// tomorrow's 08:00 digest.
	loc := time.UTC
	now := time.Date(2024, 6, 15, 10, 0, 0, 0, loc)

	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}
	// Simulate: digest was already sent and cleared today via ClearDigest.
	s.PendingDigests[blogURL] = &PendingDigest{
		SendAt: time.Date(2024, 6, 15, 8, 0, 0, 0, loc),
		Items:  []feed.Item{{GUID: "old-1"}},
	}
	s.ClearDigest(blogURL)

	// New items arrive. DigestSendAt should return tomorrow.
	sendAt := s.DigestSendAt(now, blogURL, "08:00", loc)
	s.AppendDigestItems(blogURL, []feed.Item{
		{FeedURL: blogURL, GUID: "new-1", Title: "New Post"},
	}, sendAt)

	// The pending digest should be scheduled for tomorrow 08:00, not today.
	pd := s.PendingDigests[blogURL]
	expected := time.Date(2024, 6, 16, 8, 0, 0, 0, loc)
	if !pd.SendAt.Equal(expected) {
		t.Errorf("expected sendAt %v, got %v", expected, pd.SendAt)
	}

	// It should NOT be ready now.
	ready := s.ReadyDigests(now)
	if len(ready) != 0 {
		t.Errorf("expected 0 ready digests, got %d", len(ready))
	}
}

func TestDigestFirstRunAfterDigestTimeIsImmediatelyReady(t *testing.T) {
	// Scenario: first/only run of the day at 10:00 for an 08:00 digest.
	// No digest was sent today, no pending entry exists.
	// Items should be immediately ready (sendAt = today 08:00, which is in the past).
	loc := time.UTC
	now := time.Date(2024, 6, 15, 10, 0, 0, 0, loc)

	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}

	sendAt := s.DigestSendAt(now, blogURL, "08:00", loc)
	s.AppendDigestItems(blogURL, []feed.Item{
		{FeedURL: blogURL, GUID: "post-1", Title: "Post 1"},
	}, sendAt)

	// sendAt should be today 08:00 (in the past = immediately ready).
	pd := s.PendingDigests[blogURL]
	expected := time.Date(2024, 6, 15, 8, 0, 0, 0, loc)
	if !pd.SendAt.Equal(expected) {
		t.Errorf("expected sendAt %v, got %v", expected, pd.SendAt)
	}

	// Should be ready now.
	ready := s.ReadyDigests(now)
	if _, ok := ready[blogURL]; !ok {
		t.Error("expected digest to be immediately ready on first run after digest time")
	}
}

func TestDigestItemsJoinExistingPendingDigest(t *testing.T) {
	// Scenario: items were accumulated at 07:00 (sendAt = today 08:00).
	// At 07:30, more items are fetched and should join the same pending digest.
	loc := time.UTC
	now := time.Date(2024, 6, 15, 7, 30, 0, 0, loc)

	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}
	// Earlier run accumulated items with sendAt = today 08:00.
	todaySendAt := time.Date(2024, 6, 15, 8, 0, 0, 0, loc)
	s.PendingDigests[blogURL] = &PendingDigest{
		SendAt: todaySendAt,
		Items:  []feed.Item{{FeedURL: blogURL, GUID: "old-1"}},
	}

	// New items arrive. DigestSendAt at 07:30 = today 08:00.
	sendAt := s.DigestSendAt(now, blogURL, "08:00", loc)
	s.AppendDigestItems(blogURL, []feed.Item{
		{FeedURL: blogURL, GUID: "new-1"},
	}, sendAt)

	// Should still have the original sendAt (today 08:00) with both items.
	pd := s.PendingDigests[blogURL]
	if !pd.SendAt.Equal(todaySendAt) {
		t.Errorf("expected sendAt preserved at %v, got %v", todaySendAt, pd.SendAt)
	}
	if len(pd.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(pd.Items))
	}
}

func TestDigestItemsJoinReadyPendingDigest(t *testing.T) {
	// Scenario: items were accumulated before 08:00 (sendAt = today 08:00).
	// At 08:05, before the digest is sent, more items are fetched.
	// They should join the existing pending digest (which is already ready).
	loc := time.UTC
	now := time.Date(2024, 6, 15, 8, 5, 0, 0, loc)

	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}
	// Earlier run accumulated items with sendAt = today 08:00.
	todaySendAt := time.Date(2024, 6, 15, 8, 0, 0, 0, loc)
	s.PendingDigests[blogURL] = &PendingDigest{
		SendAt: todaySendAt,
		Items:  []feed.Item{{FeedURL: blogURL, GUID: "old-1"}},
	}

	// New items arrive. Even though DigestSendAt would return tomorrow for a
	// *new* entry, AppendDigestItems preserves the existing sendAt (today 08:00).
	sendAt := s.DigestSendAt(now, blogURL, "08:00", loc)
	s.AppendDigestItems(blogURL, []feed.Item{
		{FeedURL: blogURL, GUID: "new-1"},
	}, sendAt)

	pd := s.PendingDigests[blogURL]
	if !pd.SendAt.Equal(todaySendAt) {
		t.Errorf("expected original sendAt %v preserved, got %v", todaySendAt, pd.SendAt)
	}
	if len(pd.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(pd.Items))
	}

	// The digest should be ready now (sendAt 08:00 <= now 08:05).
	ready := s.ReadyDigests(now)
	if _, ok := ready[blogURL]; !ok {
		t.Error("expected digest to be ready")
	}
}

func TestClearDigest(t *testing.T) {
	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}
	s.PendingDigests[blogURL] = &PendingDigest{
		SendAt: time.Now(),
		Items:  []feed.Item{{GUID: "a"}},
	}

	s.ClearDigest(blogURL)
	if _, ok := s.PendingDigests[blogURL]; ok {
		t.Error("expected digest to be cleared")
	}
}

func TestNextDigestTime(t *testing.T) {
	loc := time.UTC

	// Before digest time today -> returns today.
	morning := time.Date(2024, 6, 15, 6, 0, 0, 0, loc)
	result := NextDigestTime(morning, "08:00", loc)
	expected := time.Date(2024, 6, 15, 8, 0, 0, 0, loc)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}

	// After digest time today -> returns tomorrow.
	afternoon := time.Date(2024, 6, 15, 10, 0, 0, 0, loc)
	result = NextDigestTime(afternoon, "08:00", loc)
	expected = time.Date(2024, 6, 16, 8, 0, 0, 0, loc)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}

	// Exactly at digest time -> returns tomorrow.
	exact := time.Date(2024, 6, 15, 8, 0, 0, 0, loc)
	result = NextDigestTime(exact, "08:00", loc)
	expected = time.Date(2024, 6, 16, 8, 0, 0, 0, loc)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}

	// System time in UTC, digest timezone is behind: the local date in the
	// target timezone may differ from the UTC date. At 01:00 UTC on June 15,
	// it's 21:00 EDT on June 14. Since 21:00 is past 08:00, the next digest
	// is June 15 08:00 EDT. Without the In(loc) fix, the code would use the
	// UTC date (June 15) and return June 16 08:00 EDT — one day too late.
	ny, _ := time.LoadLocation("America/New_York")
	utcNow := time.Date(2024, 6, 15, 1, 0, 0, 0, time.UTC)
	result = NextDigestTime(utcNow, "08:00", ny)
	expected = time.Date(2024, 6, 15, 8, 0, 0, 0, ny)
	if !result.Equal(expected) {
		t.Errorf("cross-timezone: expected %v, got %v", expected, result)
	}
}

func TestPendingDigestsSurviveSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{
		Seen:           make(map[string]time.Time),
		KnownFeeds:     make(map[string]bool),
		PendingDigests: make(map[string]*PendingDigest),
	}
	sendAt := time.Date(2024, 6, 15, 8, 0, 0, 0, time.UTC)
	s.AppendDigestItems(blogURL, []feed.Item{
		{FeedURL: blogURL, GUID: "a", Title: "Post A"},
	}, sendAt)

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	pd, ok := loaded.PendingDigests[blogURL]
	if !ok {
		t.Fatal("expected pending digest after reload")
	}
	if len(pd.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(pd.Items))
	}
	if pd.Items[0].GUID != "a" {
		t.Errorf("expected GUID 'a', got %q", pd.Items[0].GUID)
	}
}

func TestLoadStateWithoutPendingDigests(t *testing.T) {
	// Backward compat: old state files have no pending_digests field.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte(`{"seen":{},"known_feeds":{}}`), 0o644)

	s, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.PendingDigests == nil {
		t.Error("expected PendingDigests to be initialized")
	}
	if len(s.PendingDigests) != 0 {
		t.Errorf("expected 0 pending digests, got %d", len(s.PendingDigests))
	}
}

func TestFilterNewDigestFeedReturnsAllItems(t *testing.T) {
	// A brand-new digest feed should return all items, not just the latest.
	s := &State{Seen: make(map[string]time.Time), KnownFeeds: make(map[string]bool)}

	now := time.Now()
	items := []feed.Item{
		{FeedName: "Digest Blog", FeedURL: blogURL, GUID: "a", PublishedAt: now.Add(-6 * 24 * time.Hour)},
		{FeedName: "Digest Blog", FeedURL: blogURL, GUID: "b", PublishedAt: now.Add(-1 * 24 * time.Hour)},
		{FeedName: "Digest Blog", FeedURL: blogURL, GUID: "c", PublishedAt: now.Add(-3 * 24 * time.Hour)},
	}

	digestFeeds := map[string]bool{blogURL: true}
	result := s.FilterNewItems(items, digestFeeds)
	if len(result) != 3 {
		t.Fatalf("expected 3 items for new digest feed, got %d", len(result))
	}
	// Feed should now be known.
	if !s.KnownFeeds[blogURL] {
		t.Error("expected feed to be marked as known")
	}
	// No items should be pre-marked as seen (caller handles that for digests).
	for _, item := range items {
		if s.HasSeen(blogURL, item.GUID) {
			t.Errorf("item %s should not be marked seen yet", item.GUID)
		}
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

	result := s.FilterNewItems(items, nil)
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
