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

	items, err := feed.FetchAll(ctx, cfg.Feeds, cfg.UserAgent)
	if err != nil {
		return fmt.Errorf("fetching feeds: %w", err)
	}
	slog.Info("fetched items", "count", len(items))

	newItems := st.FilterNewItems(items)
	slog.Info("new items", "count", len(newItems))

	if len(newItems) == 0 {
		return nil
	}

	// Sort oldest first so we send items in chronological order
	// and defer the newest ones when limits kick in.
	sort.Slice(newItems, func(i, j int) bool {
		return newItems[i].PublishedAt.Before(newItems[j].PublishedAt)
	})

	if limit := cfg.Email.MaxPerFeed; limit > 0 {
		newItems = applyPerFeedLimit(newItems, limit)
	}

	if limit := cfg.Email.MaxPerDay; limit > 0 {
		alreadySent := st.SendsToday()
		remaining := limit - alreadySent
		if remaining <= 0 {
			slog.Warn("daily email limit reached, skipping", "limit", limit, "already_sent", alreadySent)
			return nil
		}
		if len(newItems) > remaining {
			slog.Warn("daily limit caps this run", "limit", limit, "already_sent", alreadySent, "sending", remaining, "deferred", len(newItems)-remaining)
			newItems = newItems[:remaining]
		}
	}

	if len(newItems) == 0 {
		return nil
	}

	if dryRun {
		for _, item := range newItems {
			fmt.Printf("[%s] %s\n  %s\n\n", item.FeedName, item.Title, item.Link)
		}
		return nil
	}

	var sent int
	sender := email.NewSender(cfg.Email)
	err = sender.SendAll(newItems, func(item feed.Item) {
		sent++
		st.MarkSeen(item.FeedURL, item.GUID)
		st.RecordSend()
		if err := st.Save(statePath); err != nil {
			slog.Warn("failed to save state", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("sent %d/%d emails: %w", sent, len(newItems), err)
	}

	slog.Info("sent emails", "count", sent)
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
