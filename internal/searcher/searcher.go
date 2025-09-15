package searcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/abhijith/bookmark-cli/internal/models"
	"github.com/go-redis/redis/v8"
	"github.com/urfave/cli/v2"
)

const (
	RedisBookmarksKey = "bookmarks:index"
)

type SearchOptions struct {
	Query      string
	Tags       []string
	DateFrom   *int64
	DateTo     *int64
	Limit      int
	IncludeLLM bool
}

func SearchCommand(redisClient *redis.Client) cli.ActionFunc {
	return func(c *cli.Context) error {
		return InteractiveSearch(redisClient)
	}
}

func InteractiveSearch(redisClient *redis.Client) error {
	fmt.Println("Interactive Bookmark Search (Ctrl+C to exit)")
	fmt.Println("Shortcuts: /search, #tag, @date, !llm")
	fmt.Println("Examples:")
	fmt.Println("  /golang programming")
	fmt.Println("  #database #redis")
	fmt.Println("  @2023-01-01 @2023-12-31")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		opts := parseSearchInput(input)
		results, err := searchBookmarks(redisClient, opts)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		displayResults(results)
	}
	return nil
}

func searchBookmarks(redisClient *redis.Client, opts SearchOptions) ([]models.Bookmark, error) {
	ctx := context.Background()

	// Get all bookmarks
	zRange := redisClient.ZRangeWithScores(ctx, RedisBookmarksKey, 0, -1)
	results, err := zRange.Result()
	if err != nil {
		return nil, err
	}

	var matches []models.Bookmark
	for _, z := range results {
		var bm models.Bookmark
		if err := json.Unmarshal([]byte(z.Member.(string)), &bm); err != nil {
			continue
		}

		// Apply filters
		if !matchesFilters(bm, opts) {
			continue
		}

		matches = append(matches, bm)
		if len(matches) >= opts.Limit {
			break
		}
	}

	// Sort by relevance (simplified)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt > matches[j].CreatedAt
	})

	return matches, nil
}

func matchesFilters(bm models.Bookmark, opts SearchOptions) bool {
	// Text search
	if opts.Query != "" {
		query := strings.ToLower(opts.Query)
		title := strings.ToLower(bm.Title)
		desc := strings.ToLower(bm.Description)
		url := strings.ToLower(bm.URL)

		if !strings.Contains(title, query) &&
			!strings.Contains(desc, query) &&
			!strings.Contains(url, query) {
			return false
		}
	}

	// Tag filtering
	if len(opts.Tags) > 0 {
		tagMap := make(map[string]bool)
		for _, tag := range bm.Tags {
			tagMap[strings.ToLower(tag)] = true
		}
		for _, tag := range opts.Tags {
			if !tagMap[strings.ToLower(tag)] {
				return false
			}
		}
	}

	// Date filtering
	if opts.DateFrom != nil && bm.CreatedAt < *opts.DateFrom {
		return false
	}
	if opts.DateTo != nil && bm.CreatedAt > *opts.DateTo {
		return false
	}

	return true
}

func parseSearchInput(input string) SearchOptions {
	opts := SearchOptions{
		Limit: 20,
	}

	parts := strings.Fields(input)
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "/"):
			opts.Query = strings.TrimPrefix(part, "/")
		case strings.HasPrefix(part, "#"):
			opts.Tags = append(opts.Tags, strings.TrimPrefix(part, "#"))
		case strings.HasPrefix(part, "@"):
			dateStr := strings.TrimPrefix(part, "@")
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				if opts.DateFrom == nil {
					opts.DateFrom = &[]int64{t.Unix()}[0]
				} else {
					opts.DateTo = &[]int64{t.Unix()}[0]
				}
			}
		case strings.HasPrefix(part, "!"):
			opts.IncludeLLM = true
		default:
			// If no prefix, treat as search query
			if opts.Query == "" {
				opts.Query = part
			} else {
				opts.Query += " " + part
			}
		}
	}

	return opts
}

func displayResults(results []models.Bookmark) {
	if len(results) == 0 {
		fmt.Println("No results found")
		return
	}

	fmt.Printf("Found %d results:\n\n", len(results))
	for i, bm := range results {
		fmt.Printf("%d. %s\n", i+1, bm.Title)
		fmt.Printf("   %s\n", bm.URL)
		if bm.Description != "" {
			fmt.Printf("   %s\n", bm.Description)
		}
		if len(bm.Tags) > 0 {
			fmt.Printf("   Tags: %s\n", strings.Join(bm.Tags, ", "))
		}
		fmt.Printf("   Created: %s\n", time.Unix(bm.CreatedAt, 0).Format("2006-01-02"))
		fmt.Println()
	}
}



