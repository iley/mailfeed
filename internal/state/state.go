package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iley/mailfeed/internal/feed"
)

// MaxDigestItems is the maximum number of items in a single digest email.
// If a pending digest exceeds this, only the oldest items are sent and the
// rest are kept for the next digest cycle.
const MaxDigestItems = 50

type State struct {
	Seen           map[string]time.Time      `json:"seen"`
	KnownFeeds     map[string]bool           `json:"known_feeds"`
	DailySends     *DailySends               `json:"daily_sends,omitempty"`
	PendingDigests map[string]*PendingDigest `json:"pending_digests,omitempty"`
	DigestsSent    *DigestsSentRecord        `json:"digests_sent,omitempty"`
}

// DigestsSentRecord tracks which feeds had a digest sent on a given date,
// so we can distinguish "first run today" from "already sent today" when
// deciding whether to make new items immediately ready or defer to tomorrow.
type DigestsSentRecord struct {
	Date  string          `json:"date"`
	Feeds map[string]bool `json:"feeds"`
}

type PendingDigest struct {
	SendAt time.Time   `json:"send_at"`
	Items  []feed.Item `json:"items"`
}

type DailySends struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{
				Seen:           make(map[string]time.Time),
				KnownFeeds:     make(map[string]bool),
				PendingDigests: make(map[string]*PendingDigest),
			}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	if s.Seen == nil {
		s.Seen = make(map[string]time.Time)
	}
	if s.KnownFeeds == nil {
		s.KnownFeeds = make(map[string]bool)
	}
	if s.PendingDigests == nil {
		s.PendingDigests = make(map[string]*PendingDigest)
	}
	return &s, nil
}

const retentionPeriod = 90 * 24 * time.Hour

// Prune removes seen entries older than the retention period.
// KnownFeeds are never pruned, so new-feed detection continues to work.
func (s *State) Prune() {
	cutoff := time.Now().Add(-retentionPeriod)
	for key, ts := range s.Seen {
		if ts.Before(cutoff) {
			delete(s.Seen, key)
		}
	}
}

func (s *State) Save(path string) error {
	s.Prune()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming state: %w", err)
	}
	return nil
}

func (s *State) HasSeen(feedURL, guid string) bool {
	_, ok := s.Seen[seenKey(feedURL, guid)]
	return ok
}

func (s *State) MarkSeen(feedURL, guid string) {
	s.Seen[seenKey(feedURL, guid)] = time.Now()
}

func todayString() string {
	return time.Now().Format("2006-01-02")
}

// SendsToday returns the number of emails sent today.
func (s *State) SendsToday() int {
	if s.DailySends == nil || s.DailySends.Date != todayString() {
		return 0
	}
	return s.DailySends.Count
}

// RecordSend increments the daily send counter, resetting it if the date has changed.
func (s *State) RecordSend() {
	today := todayString()
	if s.DailySends == nil || s.DailySends.Date != today {
		s.DailySends = &DailySends{Date: today, Count: 0}
	}
	s.DailySends.Count++
}

func seenKey(feedURL, guid string) string {
	return feedURL + "\x00" + guid
}

// FilterNewItems returns items that should be sent.
// For feeds never processed before (by URL), only the latest item is
// returned — and only if it was published within the last 7 days.
// All other items are marked as seen. For known feeds, all unseen
// items are returned (even if all items have rotated out of the feed).
//
// Digest feeds (listed in digestFeeds) bypass the new-feed restriction:
// all items are returned regardless of whether the feed is new, since
// they'll be accumulated into a digest rather than sent individually.
func (s *State) FilterNewItems(items []feed.Item, digestFeeds map[string]bool) []feed.Item {
	byFeed := make(map[string][]feed.Item)
	for _, item := range items {
		byFeed[item.FeedURL] = append(byFeed[item.FeedURL], item)
	}

	var result []feed.Item
	for feedURL, feedItems := range byFeed {
		isDigest := digestFeeds[feedURL]
		if !s.KnownFeeds[feedURL] {
			s.KnownFeeds[feedURL] = true
			if isDigest {
				// Digest feeds get all unseen items even on first run,
				// since they'll be bundled rather than sent individually.
				for _, item := range feedItems {
					if !s.HasSeen(feedURL, item.GUID) {
						result = append(result, item)
					}
				}
			} else {
				latest := latestItem(feedItems)
				// For new feeds, only send the latest item if it's recent enough.
				recentEnough := time.Since(latest.PublishedAt) <= 7*24*time.Hour
				for _, item := range feedItems {
					if item.GUID != latest.GUID || !recentEnough {
						s.MarkSeen(feedURL, item.GUID)
					}
				}
				if recentEnough {
					result = append(result, latest)
				}
			}
		} else {
			for _, item := range feedItems {
				if !s.HasSeen(feedURL, item.GUID) {
					result = append(result, item)
				}
			}
		}
	}
	return result
}

// AppendDigestItems adds items to a feed's pending digest.
// If no pending digest exists for the feed, a new one is created with the given sendAt time.
// If one already exists, items are appended and the existing sendAt is preserved.
func (s *State) AppendDigestItems(feedURL string, items []feed.Item, sendAt time.Time) {
	pd, ok := s.PendingDigests[feedURL]
	if !ok {
		s.PendingDigests[feedURL] = &PendingDigest{
			SendAt: sendAt,
			Items:  items,
		}
		return
	}
	pd.Items = append(pd.Items, items...)
}

// ReadyDigests returns pending digests whose send time has arrived.
func (s *State) ReadyDigests(now time.Time) map[string]*PendingDigest {
	ready := make(map[string]*PendingDigest)
	for url, pd := range s.PendingDigests {
		if !now.Before(pd.SendAt) {
			ready[url] = pd
		}
	}
	return ready
}

// ClearDigest removes a feed's pending digest after successful send
// and records that a digest was sent today for this feed.
func (s *State) ClearDigest(feedURL string) {
	delete(s.PendingDigests, feedURL)
	s.recordDigestSent(feedURL)
}

func (s *State) recordDigestSent(feedURL string) {
	today := todayString()
	if s.DigestsSent == nil || s.DigestsSent.Date != today {
		s.DigestsSent = &DigestsSentRecord{Date: today, Feeds: make(map[string]bool)}
	}
	s.DigestsSent.Feeds[feedURL] = true
}

// DigestSentToday returns whether a digest was already sent today for this feed.
func (s *State) DigestSentToday(feedURL string) bool {
	if s.DigestsSent == nil || s.DigestsSent.Date != todayString() {
		return false
	}
	return s.DigestsSent.Feeds[feedURL]
}

// DigestSendAt returns the appropriate sendAt time for accumulating digest items.
//
// The logic handles three cases:
//   - Before digest time today: returns today at digestTime (items wait).
//   - After digest time, no digest sent today: returns today at digestTime
//     (already in the past, so items are immediately ready).
//   - After digest time, digest already sent today: returns tomorrow at digestTime
//     (prevents sending a second digest the same day).
//
// If a pending digest already exists, AppendDigestItems preserves its sendAt,
// so this value only matters when creating a new pending entry.
func (s *State) DigestSendAt(now time.Time, feedURL, digestTime string, loc *time.Location) time.Time {
	todayAt := todayDigestTime(now, digestTime, loc)
	if now.Before(todayAt) {
		// Digest time hasn't passed yet today.
		return todayAt
	}
	if s.DigestSentToday(feedURL) {
		// Already sent today — defer to tomorrow.
		return todayAt.AddDate(0, 0, 1)
	}
	// Digest time passed but nothing sent yet — make items immediately ready.
	return todayAt
}

func todayDigestTime(now time.Time, digestTime string, loc *time.Location) time.Time {
	hm, _ := time.Parse("15:04", digestTime)
	hour, min := hm.Hour(), hm.Minute()
	nowInLoc := now.In(loc)
	return time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), hour, min, 0, 0, loc)
}

// NextDigestTime computes the next future occurrence of digestTime (format "15:04") in the given location.
// If now is before that time today, returns today at that time.
// If now is at or after that time today, returns tomorrow at that time.
func NextDigestTime(now time.Time, digestTime string, loc *time.Location) time.Time {
	hm, _ := time.Parse("15:04", digestTime)
	hour, min := hm.Hour(), hm.Minute()

	nowInLoc := now.In(loc)
	today := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), hour, min, 0, 0, loc)
	if now.Before(today) {
		return today
	}
	return today.AddDate(0, 0, 1)
}

func latestItem(items []feed.Item) feed.Item {
	latest := items[0]
	for _, item := range items[1:] {
		if item.PublishedAt.After(latest.PublishedAt) {
			latest = item
		}
	}
	return latest
}
