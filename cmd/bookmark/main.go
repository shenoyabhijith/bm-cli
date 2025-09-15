package main

import (
	"fmt"
	"log"
	"os"

	"github.com/abhijith/bookmark-cli/internal/browser"
	"github.com/abhijith/bookmark-cli/internal/importer"
	"github.com/abhijith/bookmark-cli/internal/redis"
	"github.com/abhijith/bookmark-cli/internal/searcher"
	"github.com/urfave/cli/v2"
)

func main() {
	// Initialize Redis connection
	redisClient := redis.NewClient()
	defer redisClient.Close()

	app := &cli.App{
		Name:  "bc",
		Usage: "Bookmark CLI - Ultra-fast bookmark manager",
		Description: `A powerful bookmark manager with Redis backend and interactive search.

Commands:
┌─────────┬─────────────────────────────────────────────────────────────┐
│ Command │ Description                                                │
├─────────┼─────────────────────────────────────────────────────────────┤
│ import  │ Import bookmarks from JSON file                            │
│ browser │ Auto-import bookmarks from browsers (Chrome, Firefox, Safari)│
│ sync    │ Sync and deduplicate bookmarks from all browsers          │
│ search  │ Interactive search with filters and shortcuts             │
│ clean   │ Remove duplicate bookmarks                                 │
└─────────┴─────────────────────────────────────────────────────────────┘

Search Shortcuts:
┌─────────┬─────────────────────────────────────────────────────────────┐
│ Shortcut│ Description                                                │
├─────────┼─────────────────────────────────────────────────────────────┤
│ /query  │ Text search in title, description, and URL                 │
│ #tag    │ Filter by tags (e.g., #database #redis)                   │
│ @date   │ Filter by date range (e.g., @2023-01-01 @2023-12-31)      │
│ !llm    │ Enable LLM processing (future feature)                     │
└─────────┴─────────────────────────────────────────────────────────────┘

Examples:
  bc import bookmarks.json
  bc browser chrome
  bc sync
  bc search
  bc clean`,
		Commands: []*cli.Command{
			{
				Name:      "import",
				Usage:     "Import bookmarks from JSON file",
				ArgsUsage: "<file>",
				Action:    importer.ImportCommand(redisClient),
			},
			{
				Name:  "browser",
				Usage: "Import bookmarks from browser",
				Subcommands: []*cli.Command{
					{
						Name:  "chrome",
						Usage: "Import from Chrome browser",
						Action: func(c *cli.Context) error {
							importer := browser.NewBrowserImporter(redisClient)
							return importer.ImportFromChrome()
						},
					},
					{
						Name:  "firefox",
						Usage: "Import from Firefox browser",
						Action: func(c *cli.Context) error {
							importer := browser.NewBrowserImporter(redisClient)
							return importer.ImportFromFirefox()
						},
					},
					{
						Name:  "safari",
						Usage: "Import from Safari browser",
						Action: func(c *cli.Context) error {
							importer := browser.NewBrowserImporter(redisClient)
							return importer.ImportFromSafari()
						},
					},
					{
						Name:  "all",
						Usage: "Import from all available browsers",
						Action: func(c *cli.Context) error {
							importer := browser.NewBrowserImporter(redisClient)
							return importer.AutoImport()
						},
					},
					{
						Name:  "test",
						Usage: "Test import with sample Chrome bookmarks",
						Action: func(c *cli.Context) error {
							importer := browser.NewBrowserImporter(redisClient)
							return importer.ImportFromChromeTest()
						},
					},
				},
			},
			{
				Name:  "sync",
				Usage: "Sync and deduplicate bookmarks from all browsers",
				Action: func(c *cli.Context) error {
					importer := browser.NewBrowserImporter(redisClient)
					return importer.SyncBookmarks()
				},
			},
			{
				Name:   "search",
				Usage:  "Interactive search mode",
				Action: searcher.SearchCommand(redisClient),
			},
			{
				Name:   "clean",
				Usage:  "Remove duplicate bookmarks",
				Action: importer.CleanCommand(redisClient),
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() == 0 {
				fmt.Println(`A powerful bookmark manager with Redis backend and interactive search.

Commands:
┌─────────┬─────────────────────────────────────────────────────────────┐
│ Command │ Description                                                │
├─────────┼─────────────────────────────────────────────────────────────┤
│ import  │ Import bookmarks from JSON file                            │
│ browser │ Auto-import bookmarks from browsers (Chrome, Firefox, Safari)│
│ sync    │ Sync and deduplicate bookmarks from all browsers          │
│ search  │ Interactive search with filters and shortcuts             │
│ clean   │ Remove duplicate bookmarks                                 │
└─────────┴─────────────────────────────────────────────────────────────┘

Search Shortcuts:
┌─────────┬─────────────────────────────────────────────────────────────┐
│ Shortcut│ Description                                                │
├─────────┼─────────────────────────────────────────────────────────────┤
│ /query  │ Text search in title, description, and URL                 │
│ #tag    │ Filter by tags (e.g., #database #redis)                   │
│ @date   │ Filter by date range (e.g., @2023-01-01 @2023-12-31)      │
│ !llm    │ Enable LLM processing (future feature)                     │
└─────────┴─────────────────────────────────────────────────────────────┘

Examples:
  bc import bookmarks.json
  bc browser chrome
  bc sync
  bc search
  bc clean`)
				return nil
			}
			return cli.ShowAppHelp(c)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
