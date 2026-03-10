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

type State struct {
	Seen       map[string]time.Time `json:"seen"`
	KnownFeeds map[string]bool      `json:"known_feeds"`
	DailySends *DailySends          `json:"daily_sends,omitempty"`
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
				Seen:       make(map[string]time.Time),
				KnownFeeds: make(map[string]bool),
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
// returned and the rest are marked as seen. For known feeds, all unseen
// items are returned (even if all items have rotated out of the feed).
func (s *State) FilterNewItems(items []feed.Item) []feed.Item {
	byFeed := make(map[string][]feed.Item)
	for _, item := range items {
		byFeed[item.FeedURL] = append(byFeed[item.FeedURL], item)
	}

	var result []feed.Item
	for feedURL, feedItems := range byFeed {
		if !s.KnownFeeds[feedURL] {
			s.KnownFeeds[feedURL] = true
			latest := latestItem(feedItems)
			for _, item := range feedItems {
				if item.GUID != latest.GUID {
					s.MarkSeen(feedURL, item.GUID)
				}
			}
			result = append(result, latest)
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

func latestItem(items []feed.Item) feed.Item {
	latest := items[0]
	for _, item := range items[1:] {
		if item.PublishedAt.After(latest.PublishedAt) {
			latest = item
		}
	}
	return latest
}
