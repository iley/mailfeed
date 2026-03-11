package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/iley/mailfeed/internal/config"
	"github.com/iley/mailfeed/internal/email"
	"github.com/iley/mailfeed/internal/feed"
	"github.com/iley/mailfeed/internal/state"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: mailfeed <once|loop> [flags]\n")
		os.Exit(1)
	}

	subcmd := os.Args[1]
	fs := flag.NewFlagSet(subcmd, flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	statePath := fs.String("state", "state.json", "path to state file")
	dryRun := fs.Bool("dry-run", false, "fetch and print without sending")
	fs.Parse(os.Args[2:])

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	switch subcmd {
	case "once":
		cfg, err := config.Load(*configPath)
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}
		if err := runOnce(context.Background(), cfg, *statePath, *dryRun); err != nil {
			slog.Error("run failed", "error", err)
			os.Exit(1)
		}
	case "loop":
		cfg, err := config.Load(*configPath)
		if err != nil {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}
		interval, err := cfg.CheckIntervalDuration()
		if err != nil || interval <= 0 {
			slog.Error("check_interval is required for loop mode", "error", err)
			os.Exit(1)
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		runLoop(ctx, cfg, *statePath, *dryRun, interval)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: mailfeed <once|loop> [flags]\n", subcmd)
		os.Exit(1)
	}
}

func runLoop(ctx context.Context, cfg *config.Config, statePath string, dryRun bool, interval time.Duration) {
	slog.Info("starting loop", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start.
	if err := runOnce(ctx, cfg, statePath, dryRun); err != nil {
		slog.Error("run failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case <-ticker.C:
			if err := runOnce(ctx, cfg, statePath, dryRun); err != nil {
				slog.Error("run failed", "error", err)
			}
		}
	}
}

func runOnce(ctx context.Context, cfg *config.Config, statePath string, dryRun bool) error {
	st, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	loc := cfg.Location()
	now := time.Now()

	items, err := feed.FetchAll(ctx, cfg.Feeds, cfg.UserAgent)
	if err != nil {
		return fmt.Errorf("fetching feeds: %w", err)
	}
	slog.Info("fetched items", "count", len(items))

	// Build a lookup of which feed URLs are digest feeds.
	digestFeeds := make(map[string]bool)
	digestTimes := make(map[string]string)
	for _, f := range cfg.Feeds {
		if f.Digest {
			digestFeeds[f.URL] = true
			digestTimes[f.URL] = cfg.FeedDigestTime(f)
		}
	}

	newItems := st.FilterNewItems(items, digestFeeds)
	slog.Info("new items", "count", len(newItems))

	// Split new items into immediate and digest.
	var immediateItems []feed.Item
	digestItems := make(map[string][]feed.Item)
	for _, item := range newItems {
		if digestFeeds[item.FeedURL] {
			digestItems[item.FeedURL] = append(digestItems[item.FeedURL], item)
		} else {
			immediateItems = append(immediateItems, item)
		}
	}

	// Sort immediate items oldest first so we send in chronological order
	// and defer the newest ones when limits kick in.
	sort.Slice(immediateItems, func(i, j int) bool {
		return immediateItems[i].PublishedAt.Before(immediateItems[j].PublishedAt)
	})

	if limit := cfg.Email.MaxPerFeed; limit > 0 {
		immediateItems = applyPerFeedLimit(immediateItems, limit)
	}

	if limit := cfg.Email.MaxPerDay; limit > 0 {
		alreadySent := st.SendsToday()
		remaining := limit - alreadySent
		if remaining <= 0 {
			slog.Warn("daily email limit reached, skipping", "limit", limit, "already_sent", alreadySent)
			immediateItems = nil
		} else if len(immediateItems) > remaining {
			slog.Warn("daily limit caps this run", "limit", limit, "already_sent", alreadySent, "sending", remaining, "deferred", len(immediateItems)-remaining)
			immediateItems = immediateItems[:remaining]
		}
	}

	if dryRun {
		for _, item := range immediateItems {
			fmt.Printf("[%s] %s\n  %s\n\n", item.FeedName, item.Title, item.Link)
		}
		for feedURL, dItems := range digestItems {
			fmt.Printf("[%s] Digest: %d new items (pending)\n", feedURL, len(dItems))
			for _, item := range dItems {
				fmt.Printf("  + %s\n    %s\n", item.Title, item.Link)
			}
			fmt.Println()
		}
		readyDigests := st.ReadyDigests(now)
		for _, pd := range readyDigests {
			items := pd.Items
			sort.Slice(items, func(i, j int) bool {
				return items[i].PublishedAt.Before(items[j].PublishedAt)
			})
			if len(items) > state.MaxDigestItems {
				items = items[:state.MaxDigestItems]
			}
			feedName := items[0].FeedName
			fmt.Printf("[%s] Digest (%d items, ready to send)\n", feedName, len(items))
			for _, item := range items {
				fmt.Printf("  - %s\n    %s\n", item.Title, item.Link)
			}
			fmt.Println()
		}
		return nil
	}

	// Accumulate digest items into pending state, marking them as seen immediately
	// so they won't be picked up again on the next run.
	//
	// DigestSendAt picks the right sendAt based on whether the digest time has
	// passed today and whether a digest was already sent today:
	//   - Before 08:00: sendAt = today 08:00 (items wait).
	//   - After 08:00, nothing sent yet: sendAt = today 08:00 (past → immediately ready).
	//   - After 08:00, already sent today: sendAt = tomorrow 08:00 (no double-send).
	// If a pending entry already exists, AppendDigestItems preserves its sendAt.
	for feedURL, items := range digestItems {
		for _, item := range items {
			st.MarkSeen(feedURL, item.GUID)
		}
		sendAt := st.DigestSendAt(now, feedURL, digestTimes[feedURL], loc)
		st.AppendDigestItems(feedURL, items, sendAt)
		slog.Info("accumulated digest items", "feed", feedURL, "count", len(items))
	}
	if len(digestItems) > 0 {
		if err := st.Save(statePath); err != nil {
			return fmt.Errorf("saving state after digest accumulation: %w", err)
		}
	}

	// Compute ready digests after accumulation so that newly fetched
	// items are included if today's digest time has already passed.
	readyDigests := st.ReadyDigests(now)

	// Send immediate items.
	if len(immediateItems) > 0 {
		var sent int
		sender := email.NewSender(cfg.Email)
		err = sender.SendAll(immediateItems, func(item feed.Item) {
			sent++
			st.MarkSeen(item.FeedURL, item.GUID)
			st.RecordSend()
			if err := st.Save(statePath); err != nil {
				slog.Warn("failed to save state", "error", err)
			}
		})
		if err != nil {
			return fmt.Errorf("sent %d/%d emails: %w", sent, len(immediateItems), err)
		}
		slog.Info("sent emails", "count", sent)
	}

	if len(readyDigests) > 0 {
		// Build digest emails, applying the per-digest item cap.
		var digests []email.DigestEmail
		// Track which digests have overflow items to keep.
		type overflow struct {
			feedURL string
			items   []feed.Item
			sendAt  time.Time
		}
		var overflows []overflow

		// Count how many digest slots remain under the daily limit.
		// We track this locally to correctly enforce the cap when
		// multiple digests are ready simultaneously.
		digestQuota := -1 // unlimited
		if cfg.Email.MaxPerDay > 0 {
			digestQuota = cfg.Email.MaxPerDay - st.SendsToday()
			if digestQuota < 0 {
				digestQuota = 0
			}
		}

		// Sort feed URLs for deterministic ordering when quota is limited.
		readyURLs := make([]string, 0, len(readyDigests))
		for url := range readyDigests {
			readyURLs = append(readyURLs, url)
		}
		sort.Strings(readyURLs)

		for _, feedURL := range readyURLs {
			pd := readyDigests[feedURL]
			if digestQuota == 0 {
				slog.Warn("daily limit reached, skipping remaining digests")
				break
			}

			// Sort items oldest-first so the cap keeps the oldest items
			// and defers the newest.
			items := pd.Items
			sort.Slice(items, func(i, j int) bool {
				return items[i].PublishedAt.Before(items[j].PublishedAt)
			})

			var extra []feed.Item
			if len(items) > state.MaxDigestItems {
				extra = items[state.MaxDigestItems:]
				items = items[:state.MaxDigestItems]
			}

			digests = append(digests, email.DigestEmail{
				FeedName: items[0].FeedName,
				FeedURL:  feedURL,
				Items:    items,
			})
			if digestQuota > 0 {
				digestQuota--
			}
			if len(extra) > 0 {
				nextSend := state.NextDigestTime(now, digestTimes[feedURL], loc)
				overflows = append(overflows, overflow{feedURL, extra, nextSend})
			}
		}

		if len(digests) > 0 {
			sender := email.NewSender(cfg.Email)
			var digestsSent int
			err = sender.SendDigests(digests, func(feedURL string) {
				digestsSent++
				st.ClearDigest(feedURL)
				st.RecordSend()
				if err := st.Save(statePath); err != nil {
					slog.Warn("failed to save state", "error", err)
				}
			})
			if err != nil {
				// Re-add overflow items before returning, so items from
				// successfully-sent digests aren't lost.
				for _, o := range overflows {
					if _, still := st.PendingDigests[o.feedURL]; !still {
						st.AppendDigestItems(o.feedURL, o.items, o.sendAt)
					}
				}
				if saveErr := st.Save(statePath); saveErr != nil {
					slog.Warn("failed to save state after partial digest send", "error", saveErr)
				}
				return fmt.Errorf("sent %d/%d digests: %w", digestsSent, len(digests), err)
			}
			slog.Info("sent digests", "count", digestsSent)
		}

		// Re-add overflow items for the next cycle.
		for _, o := range overflows {
			if _, still := st.PendingDigests[o.feedURL]; !still {
				st.AppendDigestItems(o.feedURL, o.items, o.sendAt)
			}
		}
		if err := st.Save(statePath); err != nil {
			return fmt.Errorf("saving state after digests: %w", err)
		}
	}

	return nil
}

// applyPerFeedLimit caps the number of items per feed, keeping the oldest
// ones (which come first since items are sorted by PublishedAt ascending).
// Items beyond the limit are simply omitted — they remain unseen in state
// and will be picked up on the next run.
func applyPerFeedLimit(items []feed.Item, limit int) []feed.Item {
	counts := make(map[string]int)
	var result []feed.Item
	for _, item := range items {
		if counts[item.FeedURL] < limit {
			result = append(result, item)
			counts[item.FeedURL]++
		} else if counts[item.FeedURL] == limit {
			// Log once per feed when we start dropping items.
			slog.Warn("per-feed limit reached, deferring items", "feed", item.FeedName, "limit", limit)
			counts[item.FeedURL]++
		}
	}
	return result
}
