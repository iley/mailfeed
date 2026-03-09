package feed

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
	"mailfeed/config"
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
	for _, f := range feeds {
		items, err := Fetch(f.URL, f.Name)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", f.URL, err)
		}
		all = append(all, items...)
	}
	return all, nil
}

func Fetch(url, feedName string) ([]Item, error) {
	resp, err := http.Get(url)
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
