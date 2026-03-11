package feed

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/iley/mailfeed/internal/config"

	"github.com/mmcdole/gofeed"
)

type Item struct {
	FeedName    string    `json:"feed_name"`
	FeedURL     string    `json:"feed_url"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	GUID        string    `json:"guid"`
}

func FetchAll(ctx context.Context, feeds []config.Feed, userAgent string) ([]Item, error) {
	var all []Item
	var failed int
	for _, f := range feeds {
		items, err := Fetch(ctx, f.URL, f.Name, f.URL, userAgent)
		if err != nil {
			slog.Warn("skipping feed", "url", f.URL, "error", err)
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

const maxResponseBytes = 10 << 20 // 10 MB

const (
	maxRetries     = 3
	retryBaseDelay = 1 * time.Second
)

func Fetch(ctx context.Context, url, feedName, feedURL, userAgent string) ([]Item, error) {
	body, err := fetchHTTP(ctx, url, userAgent)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return Parse(io.LimitReader(body, maxResponseBytes), feedName, feedURL)
}

// fetchHTTP fetches a URL with retry logic for transient errors.
// Returns the response body on success; the caller must close it.
func fetchHTTP(ctx context.Context, url, userAgent string) (io.ReadCloser, error) {
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			slog.Warn("retrying feed fetch", "url", url, "attempt", attempt+1, "error", lastErr)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		if userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP GET: %w", err)
			continue // network errors are retryable
		}

		if resp.StatusCode == http.StatusOK {
			return resp.Body, nil
		}
		resp.Body.Close()

		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)

		// Only retry on 429 or 5xx; other status codes are permanent failures.
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func Parse(r io.Reader, feedName, feedURL string) ([]Item, error) {
	fp := gofeed.NewParser()
	parsed, err := fp.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing feed: %w", err)
	}

	items := make([]Item, 0, len(parsed.Items))
	for _, gi := range parsed.Items {
		items = append(items, mapItem(gi, feedName, feedURL))
	}
	return items, nil
}

func mapItem(gi *gofeed.Item, feedName, feedURL string) Item {
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
		FeedURL:     feedURL,
		Title:       gi.Title,
		Link:        gi.Link,
		Content:     content,
		PublishedAt: published,
		GUID:        guid,
	}
}
