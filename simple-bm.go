package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/urfave/cli/v2"
)

var ctx = context.Background()
var redisClient *redis.Client
var version = "v0.1.0"

const (
	RedisBookmarksKey       = "bookmarks:index"
	RedisURLSetKey          = "bookmarks:urls"
	RedisBookmarksActiveKey = "bookmarks:active"
	RedisBookmarksDeadKey   = "bookmarks:dead"
	RedisURLSetActive       = "bookmarks:urls:active"
	RedisURLSetDead         = "bookmarks:urls:dead"
)

type Bookmark struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
	ID          string   `json:"id"`
	Status      string   `json:"status"` // "active", "dead", "unknown"
}

func main() {
	// Initialize Redis
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "192.168.1.100:6379",
		Password: "",
		DB:       1,
	})

	// Test connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	app := &cli.App{
		Name:    "bm",
		Usage:   "Simple bookmark manager - check duplicates and dead links",
		Version: version,
		Description: `Power-packed 6-command CLI.

Commands:
  import  — Import from browsers (zen/safari/arc/all); dedupe on ingest
  check   — Detect duplicates and classify dead links in parallel
  clean   — Remove duplicates; rebuild active/dead indices
  list    — Show active bookmarks (optionally include dead)
  search  — Full-text over title/description with filters
  dead    — Manage dead links (show/purge/revive)

Examples:
  bm import --browser zen
  bm check --concurrency 50 --timeout 6s
  bm clean
  bm list --limit 30 --tag aws --include-dead
  bm search --q "redis" --tag db --limit 20
  bm dead show | bm dead purge | bm dead revive https://example.com`,
		CustomAppHelpTemplate: `NAME:
   {{.HelpName}} - {{.Usage}}

USAGE:
   {{.HelpName}} [global options] command [command options] [arguments...]

DESCRIPTION:
   {{.Description}}

COMMANDS:
{{range .Commands}}{{if not .HideHelp}}   {{join .Names ", "}}	{{.Usage}}
{{end}}{{end}}

GLOBAL OPTIONS:
{{range .VisibleFlags}}   {{.}}
{{end}}`,
		Commands: []*cli.Command{
			{
				Name:      "import",
				Usage:     "Import bookmarks from browser",
				UsageText: "bm import [--browser zen|safari|arc|all]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "browser", Value: "all", Usage: "choose browser source: zen|safari|arc|all"},
				},
				Action: importBookmarks,
			},
			{
				Name:      "check",
				Usage:     "Check for duplicates and dead links",
				UsageText: "bm check [--concurrency N] [--timeout 8s]",
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "concurrency", Value: 20, Usage: "parallel URL checks"},
					&cli.DurationFlag{Name: "timeout", Value: 8 * time.Second, Usage: "HTTP timeout per request"},
				},
				Action: checkBookmarks,
			},
			{
				Name:      "clean",
				Usage:     "Remove duplicates and dead links (rebuild indices)",
				UsageText: "bm clean [--concurrency N] [--timeout 8s]",
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "concurrency", Value: 20, Usage: "parallel URL checks while cleaning"},
					&cli.DurationFlag{Name: "timeout", Value: 8 * time.Second, Usage: "HTTP timeout per request"},
				},
				Action: cleanBookmarks,
			},
			{
				Name:      "list",
				Usage:     "List bookmarks (active by default)",
				UsageText: "bm list [--limit N] [--tag t1 --tag t2] [--include-dead]",
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "limit", Value: 50, Usage: "max items"},
					&cli.StringSliceFlag{Name: "tag", Usage: "filter by tag (repeatable)"},
					&cli.BoolFlag{Name: "include-dead", Usage: "include dead bookmarks"},
				},
				Action: listBookmarks,
			},
			{
				Name:      "search",
				Usage:     "Search bookmarks",
				UsageText: "bm search [--q query] [--tag t1 --tag t2] [--limit N] [--include-dead]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "q", Usage: "query string"},
					&cli.StringSliceFlag{Name: "tag", Usage: "filter by tag (repeatable)"},
					&cli.StringFlag{Name: "from", Usage: "date from (YYYY-MM-DD)"},
					&cli.StringFlag{Name: "to", Usage: "date to (YYYY-MM-DD)"},
					&cli.IntFlag{Name: "limit", Value: 50, Usage: "max items"},
					&cli.BoolFlag{Name: "include-dead", Usage: "include dead bookmarks"},
				},
				Action: searchBookmarksCmd,
			},
			{
				Name:  "dead",
				Usage: "Manage dead links",
				Subcommands: []*cli.Command{
					{Name: "show", Usage: "List dead bookmarks", UsageText: "bm dead show", Action: deadShowCmd},
					{Name: "purge", Usage: "Delete all dead bookmarks index", UsageText: "bm dead purge", Action: deadPurgeCmd},
					{Name: "revive", Usage: "Move URL from dead to active", UsageText: "bm dead revive <url>", ArgsUsage: "<url>", Action: deadReviveCmd},
				},
			},
		},
	}

	// Default action: print help when no command is provided
	app.Action = func(c *cli.Context) error { return cli.ShowAppHelp(c) }

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func checkBookmarks(c *cli.Context) error {
	fmt.Println("🔍 Checking bookmarks for duplicates and dead links...")

	// Get all bookmarks
	bookmarks, err := getAllBookmarks()
	if err != nil {
		return err
	}

	if len(bookmarks) == 0 {
		fmt.Println("No bookmarks found")
		return nil
	}

	fmt.Printf("Found %d bookmarks\n\n", len(bookmarks))

	// Check for duplicates
	fmt.Println("🔍 Checking for duplicates...")
	duplicates := findDuplicates(bookmarks)
	if len(duplicates) > 0 {
		fmt.Printf("❌ Found %d duplicate URLs:\n", len(duplicates))
		for _, dup := range duplicates {
			fmt.Printf("  - %s (%d occurrences)\n", dup.URL, dup.Count)
		}
		fmt.Println()
	} else {
		fmt.Println("✅ No duplicates found")
	}

	// Check for dead links
	fmt.Println("🌐 Checking website health...")
	concurrency := c.Int("concurrency")
	timeout := c.Duration("timeout")
	deadLinks := checkDeadLinks(bookmarks, concurrency, timeout)
	if len(deadLinks) > 0 {
		fmt.Printf("❌ Found %d dead links:\n", len(deadLinks))
		for _, link := range deadLinks {
			fmt.Printf("  - %s (%s)\n", link.Title, link.URL)
		}
		fmt.Println()
	} else {
		fmt.Println("✅ All links are active")
	}

	fmt.Printf("Summary: %d total, %d duplicates, %d dead links\n",
		len(bookmarks), len(duplicates), len(deadLinks))

	return nil
}

func cleanBookmarks(c *cli.Context) error {
	fmt.Println("🧹 Cleaning bookmarks...")

	// Get all bookmarks
	bookmarks, err := getAllBookmarks()
	if err != nil {
		return err
	}

	if len(bookmarks) == 0 {
		fmt.Println("No bookmarks to clean")
		return nil
	}

	// Remove duplicates (keep first occurrence)
	uniqueBookmarks := removeDuplicates(bookmarks)
	fmt.Printf("Removed %d duplicate bookmarks\n", len(bookmarks)-len(uniqueBookmarks))

	// Check and remove dead links
	fmt.Println("Checking for dead links...")
	concurrency := c.Int("concurrency")
	timeout := c.Duration("timeout")
	activeBookmarks := removeDeadLinks(uniqueBookmarks, concurrency, timeout)
	fmt.Printf("Removed %d dead bookmarks\n", len(uniqueBookmarks)-len(activeBookmarks))

	// Save cleaned bookmarks back to Redis
	if err := saveBookmarksClassified(activeBookmarks); err != nil {
		return err
	}

	fmt.Printf("✅ Cleanup complete: %d bookmarks remaining\n", len(activeBookmarks))
	return nil
}

func listBookmarks(c *cli.Context) error {
	includeDead := c.Bool("include-dead")

	var bookmarks []Bookmark
	var err error
	if includeDead {
		active, err1 := getFromZSet(RedisBookmarksActiveKey)
		dead, err2 := getFromZSet(RedisBookmarksDeadKey)
		if err1 != nil {
			return err1
		}
		if err2 != nil {
			return err2
		}
		bookmarks = append(active, dead...)
	} else {
		bookmarks, err = getFromZSet(RedisBookmarksActiveKey)
		if err != nil {
			return err
		}
	}

	if len(bookmarks) == 0 {
		fmt.Println("No bookmarks found")
		return nil
	}

	fmt.Printf("📚 %d bookmarks:\n\n", len(bookmarks))
	for i, bm := range bookmarks {
		fmt.Printf("%d. %s\n", i+1, bm.Title)
		fmt.Printf("   %s\n", bm.URL)
		if bm.Description != "" {
			fmt.Printf("   %s\n", bm.Description)
		}
		if len(bm.Tags) > 0 {
			fmt.Printf("   Tags: %v\n", bm.Tags)
		}
		fmt.Println()
	}

	return nil
}

func getFromZSet(zkey string) ([]Bookmark, error) {
	zRange := redisClient.ZRangeWithScores(ctx, zkey, 0, -1)
	results, err := zRange.Result()
	if err != nil {
		return nil, err
	}
	var bookmarks []Bookmark
	for _, z := range results {
		var bm Bookmark
		if err := json.Unmarshal([]byte(z.Member.(string)), &bm); err != nil {
			continue
		}
		bookmarks = append(bookmarks, bm)
	}
	return bookmarks, nil
}

func importBookmarks(c *cli.Context) error {
	fmt.Println("📥 Importing bookmarks from browsers...")

	// This would integrate with your existing browser import logic
	// For now, just show a message
	fmt.Println("Browser import functionality will be integrated here")

	return nil
}

// Helper functions

func getAllBookmarks() ([]Bookmark, error) {
	zRange := redisClient.ZRangeWithScores(ctx, RedisBookmarksKey, 0, -1)
	results, err := zRange.Result()
	if err != nil {
		return nil, err
	}

	var bookmarks []Bookmark
	for _, z := range results {
		var bm Bookmark
		if err := json.Unmarshal([]byte(z.Member.(string)), &bm); err != nil {
			continue
		}
		bookmarks = append(bookmarks, bm)
	}

	return bookmarks, nil
}

func findDuplicates(bookmarks []Bookmark) []DuplicateInfo {
	urlCount := make(map[string]int)

	for _, bm := range bookmarks {
		urlCount[bm.URL]++
	}

	var duplicates []DuplicateInfo
	for url, count := range urlCount {
		if count > 1 {
			duplicates = append(duplicates, DuplicateInfo{
				URL:   url,
				Count: count,
			})
		}
	}

	return duplicates
}

func removeDuplicates(bookmarks []Bookmark) []Bookmark {
	seen := make(map[string]bool)
	var unique []Bookmark

	for _, bm := range bookmarks {
		if !seen[bm.URL] {
			seen[bm.URL] = true
			unique = append(unique, bm)
		}
	}

	return unique
}

func checkDeadLinks(bookmarks []Bookmark, concurrency int, timeout time.Duration) []Bookmark {
	var (
		deadLinks []Bookmark
		deadMutex sync.Mutex
		progress  int64
	)

	client := &http.Client{Timeout: timeout}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, bm := range bookmarks {
		wg.Add(1)
		sem <- struct{}{}
		go func(b Bookmark) {
			defer wg.Done()
			defer func() { <-sem }()

			ok := urlHealthy(client, b.URL)
			if !ok {
				deadMutex.Lock()
				deadLinks = append(deadLinks, b)
				deadMutex.Unlock()
			}

			p := atomic.AddInt64(&progress, 1)
			fmt.Printf("\rChecking %d/%d ...", p, len(bookmarks))
		}(bm)
	}

	wg.Wait()
	fmt.Printf("\rChecked %d/%d: done.               \n", len(bookmarks), len(bookmarks))
	return deadLinks
}

func removeDeadLinks(bookmarks []Bookmark, concurrency int, timeout time.Duration) []Bookmark {
	var (
		keep     = make([]bool, len(bookmarks))
		progress int64
	)

	client := &http.Client{Timeout: timeout}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, bm := range bookmarks {
		i, bm := i, bm
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if urlHealthy(client, bm.URL) {
				keep[i] = true
			}

			p := atomic.AddInt64(&progress, 1)
			fmt.Printf("\rValidating %d/%d ...", p, len(bookmarks))
		}()
	}

	wg.Wait()
	fmt.Printf("\rValidated %d/%d: done.              \n", len(bookmarks), len(bookmarks))

	var activeBookmarks []Bookmark
	for i, k := range keep {
		if k {
			activeBookmarks = append(activeBookmarks, bookmarks[i])
		}
	}
	return activeBookmarks
}

// urlHealthy performs a HEAD request, falling back to GET if needed
func urlHealthy(client *http.Client, rawURL string) bool {
	resp, err := client.Head(rawURL)
	if err == nil && resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			return true
		}
	}
	// Some servers do not implement HEAD correctly; try GET with Range to minimize body
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err = client.Do(req)
	if err != nil || resp == nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
}

func saveBookmarks(bookmarks []Bookmark) error {
	// Clear existing bookmarks
	redisClient.Del(ctx, RedisBookmarksKey)
	redisClient.Del(ctx, RedisURLSetKey)

	// Add cleaned bookmarks
	for _, bm := range bookmarks {
		jsonData, _ := json.Marshal(bm)
		redisClient.ZAdd(ctx, RedisBookmarksKey, &redis.Z{
			Score:  float64(bm.CreatedAt),
			Member: jsonData,
		})
		redisClient.SAdd(ctx, RedisURLSetKey, bm.URL)
	}

	return nil
}

func saveBookmarksClassified(active []Bookmark) error {
	// Read current full set to identify dead
	all, err := getAllBookmarks()
	if err != nil {
		return err
	}

	// Build active and dead sets by URL
	activeURL := make(map[string]bool)
	for _, bm := range active {
		activeURL[bm.URL] = true
	}

	var dead []Bookmark
	for _, bm := range all {
		if !activeURL[bm.URL] {
			dead = append(dead, bm)
		}
	}

	// Replace indices
	redisClient.Del(ctx, RedisBookmarksActiveKey)
	redisClient.Del(ctx, RedisBookmarksDeadKey)
	redisClient.Del(ctx, RedisURLSetActive)
	redisClient.Del(ctx, RedisURLSetDead)

	for _, bm := range active {
		jsonData, _ := json.Marshal(bm)
		redisClient.ZAdd(ctx, RedisBookmarksActiveKey, &redis.Z{Score: float64(bm.CreatedAt), Member: jsonData})
		redisClient.SAdd(ctx, RedisURLSetActive, bm.URL)
	}
	for _, bm := range dead {
		jsonData, _ := json.Marshal(bm)
		redisClient.ZAdd(ctx, RedisBookmarksDeadKey, &redis.Z{Score: float64(bm.CreatedAt), Member: jsonData})
		redisClient.SAdd(ctx, RedisURLSetDead, bm.URL)
	}

	// Also rebuild the main combined index to only include active
	redisClient.Del(ctx, RedisBookmarksKey)
	redisClient.Del(ctx, RedisURLSetKey)
	for _, bm := range active {
		jsonData, _ := json.Marshal(bm)
		redisClient.ZAdd(ctx, RedisBookmarksKey, &redis.Z{Score: float64(bm.CreatedAt), Member: jsonData})
		redisClient.SAdd(ctx, RedisURLSetKey, bm.URL)
	}

	return nil
}

func searchBookmarksCmd(c *cli.Context) error {
	q := c.String("q")
	tags := c.StringSlice("tag")
	includeDead := c.Bool("include-dead")
	limit := c.Int("limit")

	active, _ := getFromZSet(RedisBookmarksActiveKey)
	var pool []Bookmark = active
	if includeDead {
		dead, _ := getFromZSet(RedisBookmarksDeadKey)
		pool = append(pool, dead...)
	}

	filtered := filterByQueryTags(pool, q, tags)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	printBookmarks(filtered)
	return nil
}

func filterByQueryTags(items []Bookmark, q string, tags []string) []Bookmark {
	q = strings.ToLower(strings.TrimSpace(q))
	wantTags := map[string]bool{}
	for _, t := range tags {
		wantTags[strings.ToLower(t)] = true
	}

	var out []Bookmark
	for _, bm := range items {
		if q != "" {
			if !strings.Contains(strings.ToLower(bm.Title), q) && !strings.Contains(strings.ToLower(bm.Description), q) {
				continue
			}
		}
		if len(wantTags) > 0 {
			ok := true
			for wt := range wantTags {
				found := false
				for _, t := range bm.Tags {
					if strings.ToLower(t) == wt {
						found = true
						break
					}
				}
				if !found {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, bm)
	}
	return out
}

func printBookmarks(items []Bookmark) {
	if len(items) == 0 {
		fmt.Println("No bookmarks found")
		return
	}
	fmt.Printf("📚 %d bookmarks:\n\n", len(items))
	for i, bm := range items {
		fmt.Printf("%d. %s\n", i+1, bm.Title)
		fmt.Printf("   %s\n", bm.URL)
		if bm.Description != "" {
			fmt.Printf("   %s\n", bm.Description)
		}
		if len(bm.Tags) > 0 {
			fmt.Printf("   Tags: %v\n", bm.Tags)
		}
		fmt.Println()
	}
}

func deadShowCmd(c *cli.Context) error {
	dead, err := getFromZSet(RedisBookmarksDeadKey)
	if err != nil {
		return err
	}
	printBookmarks(dead)
	return nil
}

func deadPurgeCmd(c *cli.Context) error {
	redisClient.Del(ctx, RedisBookmarksDeadKey)
	redisClient.Del(ctx, RedisURLSetDead)
	fmt.Println("Deleted dead bookmarks index")
	return nil
}

func deadReviveCmd(c *cli.Context) error {
	if c.NArg() < 1 {
		return cli.Exit("missing <url>", 1)
	}
	url := c.Args().First()
	dead, _ := getFromZSet(RedisBookmarksDeadKey)
	for _, bm := range dead {
		if bm.URL == url {
			// move to active
			jsonData, _ := json.Marshal(bm)
			redisClient.ZAdd(ctx, RedisBookmarksActiveKey, &redis.Z{Score: float64(bm.CreatedAt), Member: jsonData})
			redisClient.SAdd(ctx, RedisURLSetActive, bm.URL)
			// remove from dead
			redisClient.SRem(ctx, RedisURLSetDead, bm.URL)
			// rebuild combined actives index as well
			redisClient.ZAdd(ctx, RedisBookmarksKey, &redis.Z{Score: float64(bm.CreatedAt), Member: jsonData})
			fmt.Println("Revived:", url)
			return nil
		}
	}
	return cli.Exit("url not found in dead list", 1)
}

type DuplicateInfo struct {
	URL   string
	Count int
}
