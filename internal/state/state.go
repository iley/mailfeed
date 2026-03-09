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

func (s *State) Save(path string) error {
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

func (s *State) HasSeen(guid string) bool {
	_, ok := s.Seen[guid]
	return ok
}

func (s *State) MarkSeen(guid string) {
	s.Seen[guid] = time.Now()
}

// FilterNewItems returns items that should be sent.
// For feeds never processed before, only the latest item is returned
// and the rest are marked as seen. For known feeds, all unseen items
// are returned (even if all items have rotated out of the feed).
func (s *State) FilterNewItems(items []feed.Item) []feed.Item {
	byFeed := make(map[string][]feed.Item)
	for _, item := range items {
		byFeed[item.FeedName] = append(byFeed[item.FeedName], item)
	}

	var result []feed.Item
	for feedName, feedItems := range byFeed {
		if !s.KnownFeeds[feedName] {
			s.KnownFeeds[feedName] = true
			latest := latestItem(feedItems)
			for _, item := range feedItems {
				if item.GUID != latest.GUID {
					s.MarkSeen(item.GUID)
				}
			}
			result = append(result, latest)
		} else {
			for _, item := range feedItems {
				if !s.HasSeen(item.GUID) {
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
