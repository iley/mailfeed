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
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	items, err := feed.FetchAll(cfg.Feeds)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Fetched %d items:\n\n", len(items))
	for _, item := range items {
		fmt.Printf("[%s] %s\n  %s\n  %s\n\n", item.FeedName, item.Title, item.Link, item.PublishedAt.Format("2006-01-02 15:04"))
	}

	if len(items) > 0 {
		html, err := email.RenderHTML(items[0])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("--- HTML preview of first item ---")
		fmt.Println(html)
	}
}
