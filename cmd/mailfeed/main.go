package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/iley/mailfeed/internal/config"
	"github.com/iley/mailfeed/internal/email"
	"github.com/iley/mailfeed/internal/feed"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "fetch feeds and render emails without sending")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	items, err := feed.FetchAll(cfg.Feeds)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Fetched %d items", len(items))

	if len(items) == 0 {
		return
	}

	if *dryRun {
		for _, item := range items {
			fmt.Printf("[%s] %s\n  %s\n\n", item.FeedName, item.Title, item.Link)
		}
		return
	}

	sender := email.NewSender(cfg.Email)
	if err := sender.SendAll(items); err != nil {
		log.Fatal(err)
	}

	log.Printf("Sent %d emails", len(items))
}
