package feed

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/iley/mailfeed/internal/config"

	"github.com/mmcdole/gofeed"
)

type Item struct {
	FeedName    string
	Title       string
	Link        string
	Content     string
	PublishedAt time.Time
	GUID        string
}

func FetchAll(feeds []config.Feed) ([]Item, error) {
	var all []Item
	var failed int
	for _, f := range feeds {
		items, err := Fetch(f.URL, f.Name)
		if err != nil {
			log.Printf("WARNING: skipping feed %s: %v", f.URL, err)
			failed++
			continue
		}
		all = append(all, items...)
	}
	if failed == len(feeds) {
		return nil, fmt.Errorf("all %d feeds failed", failed)
	}
	return all, nil
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func Fetch(url, feedName string) ([]Item, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return Parse(resp.Body, feedName)
}

func Parse(r io.Reader, feedName string) ([]Item, error) {
	fp := gofeed.NewParser()
	parsed, err := fp.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing feed: %w", err)
	}

	items := make([]Item, 0, len(parsed.Items))
	for _, gi := range parsed.Items {
		items = append(items, mapItem(gi, feedName))
	}
	return items, nil
}

func mapItem(gi *gofeed.Item, feedName string) Item {
	content := gi.Content
	if content == "" {
		content = gi.Description
	}

	guid := gi.GUID
	if guid == "" {
		guid = gi.Link
	}
	if guid == "" {
		h := sha256.Sum256([]byte(feedName + "\x00" + gi.Title + "\x00" + content))
		guid = fmt.Sprintf("sha256:%x", h[:12])
	}

	var published time.Time
	if gi.PublishedParsed != nil {
		published = *gi.PublishedParsed
	} else if gi.UpdatedParsed != nil {
		published = *gi.UpdatedParsed
	}

	return Item{
		FeedName:    feedName,
		Title:       gi.Title,
		Link:        gi.Link,
		Content:     content,
		PublishedAt: published,
		GUID:        guid,
	}
}
