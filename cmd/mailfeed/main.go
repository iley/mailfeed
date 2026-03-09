package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/iley/mailfeed/internal/config"
	"github.com/iley/mailfeed/internal/email"
	"github.com/iley/mailfeed/internal/feed"
	"github.com/iley/mailfeed/internal/state"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	statePath := flag.String("state", "state.json", "path to state file")
	dryRun := flag.Bool("dry-run", false, "fetch feeds and render emails without sending")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	st, err := state.Load(*statePath)
	if err != nil {
		log.Fatal(err)
	}

	items, err := feed.FetchAll(cfg.Feeds)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Fetched %d items", len(items))

	newItems := st.FilterNewItems(items)
	log.Printf("%d new items to send", len(newItems))

	if len(newItems) == 0 {
		return
	}

	if *dryRun {
		for _, item := range newItems {
			fmt.Printf("[%s] %s\n  %s\n\n", item.FeedName, item.Title, item.Link)
		}
		return
	}

	var sent int
	sender := email.NewSender(cfg.Email)
	err = sender.SendAll(newItems, func(item feed.Item) {
		sent++
		st.MarkSeen(item.GUID)
		if err := st.Save(*statePath); err != nil {
			log.Printf("WARNING: failed to save state: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Sent %d/%d emails, errors: %v", sent, len(newItems), err)
	}

	log.Printf("Sent %d emails", sent)
}
