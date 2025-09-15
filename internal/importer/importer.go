package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/abhijith/bookmark-cli/internal/models"
	"github.com/go-redis/redis/v8"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"
	"github.com/urfave/cli/v2"
)

const (
	RedisBookmarksKey = "bookmarks:index"
	RedisURLSetKey    = "bookmarks:urls"
	RedisTitleSetKey  = "bookmarks:titles"
)

func ImportCommand(redisClient *redis.Client) cli.ActionFunc {
	return func(c *cli.Context) error {
		if c.NArg() < 1 {
			return cli.Exit("Missing file argument", 1)
		}

		filePath := c.Args().Get(0)
		return ImportBookmarks(redisClient, filePath)
	}
}

func CleanCommand(redisClient *redis.Client) cli.ActionFunc {
	return func(c *cli.Context) error {
		return CleanDuplicates(redisClient)
	}
}

func ImportBookmarks(redisClient *redis.Client, filePath string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	bookmarks := gjson.GetBytes(data, "bookmarks").Array()
	if len(bookmarks) == 0 {
		return fmt.Errorf("no bookmarks found in file")
	}

	bar := progressbar.Default(int64(len(bookmarks)), "Importing")
	ctx := context.Background()

	imported := 0
	skipped := 0

	for _, item := range bookmarks {
		bm := models.Bookmark{
			URL:         item.Get("url").String(),
			Title:       item.Get("title").String(),
			Description: item.Get("description").String(),
			CreatedAt:   item.Get("created_at").Int(),
			UpdatedAt:   time.Now().Unix(),
		}

		// Generate unique ID
		bm.ID = generateID(bm.URL)

		// Parse tags
		tags := item.Get("tags").Array()
		for _, tag := range tags {
			bm.Tags = append(bm.Tags, tag.String())
		}

		// Deduplicate using Redis set
		exists, err := redisClient.SAdd(ctx, RedisURLSetKey, bm.URL).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			skipped++
			bar.Add(1)
			continue // Skip duplicates
		}

		// Add to search index
		jsonData, _ := json.Marshal(bm)
		if err := redisClient.ZAdd(ctx, RedisBookmarksKey, &redis.Z{
			Score:  float64(bm.CreatedAt),
			Member: jsonData,
		}).Err(); err != nil {
			return err
		}

		// Index title terms
		terms := strings.Fields(strings.ToLower(bm.Title))
		for _, term := range terms {
			redisClient.SAdd(ctx, RedisTitleSetKey, term)
		}

		imported++
		bar.Add(1)
	}

	bar.Finish()
	fmt.Printf("Import complete: %d imported, %d skipped\n", imported, skipped)
	return nil
}

func CleanDuplicates(redisClient *redis.Client) error {
	ctx := context.Background()

	// Get all URLs and remove duplicates
	urls, err := redisClient.SMembers(ctx, RedisURLSetKey).Result()
	if err != nil {
		return err
	}

	fmt.Printf("Found %d unique URLs\n", len(urls))

	// Clear the URL set and rebuild
	redisClient.Del(ctx, RedisURLSetKey)

	// Rebuild URL set with unique URLs
	for _, url := range urls {
		redisClient.SAdd(ctx, RedisURLSetKey, url)
	}

	fmt.Println("Duplicate cleanup complete")
	return nil
}

func generateID(url string) string {
	// Simple ID generation - replace with proper UUID in production
	h := fnv.New64a()
	h.Write([]byte(url))
	return strconv.FormatUint(h.Sum64(), 16)
}



