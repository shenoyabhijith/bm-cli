package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/abhijith/bookmark-cli/internal/models"
	"github.com/go-redis/redis/v8"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"
)

const (
	RedisBookmarksKey = "bookmarks:index"
	RedisURLSetKey    = "bookmarks:urls"
	RedisTitleSetKey  = "bookmarks:titles"
	RedisLastSyncKey  = "bookmarks:last_sync"
)

// BrowserBookmark represents a bookmark from browser export
type BrowserBookmark struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	CreatedAt   int64    `json:"created_at"`
	Folder      string   `json:"folder"`
}

// BrowserImporter handles browser bookmark imports
type BrowserImporter struct {
	redisClient *redis.Client
}

// NewBrowserImporter creates a new browser importer
func NewBrowserImporter(redisClient *redis.Client) *BrowserImporter {
	return &BrowserImporter{
		redisClient: redisClient,
	}
}

// ImportFromChrome imports bookmarks from Chrome browser
func (bi *BrowserImporter) ImportFromChrome() error {
	chromePath := bi.getChromeBookmarkPath()
	if chromePath == "" {
		return fmt.Errorf("Chrome bookmark file not found")
	}

	return bi.importFromFile(chromePath, "Chrome")
}

// ImportFromChromeTest imports bookmarks from test Chrome file
func (bi *BrowserImporter) ImportFromChromeTest() error {
	return bi.importFromFile("test-chrome-bookmarks.json", "Chrome")
}

// ImportFromFirefox imports bookmarks from Firefox browser
func (bi *BrowserImporter) ImportFromFirefox() error {
	firefoxPath := bi.getFirefoxBookmarkPath()
	if firefoxPath == "" {
		return fmt.Errorf("Firefox bookmark file not found")
	}

	return bi.importFromFile(firefoxPath, "Firefox")
}

// ImportFromSafari imports bookmarks from Safari browser
func (bi *BrowserImporter) ImportFromSafari() error {
	safariPath := bi.getSafariBookmarkPath()
	if safariPath == "" {
		return fmt.Errorf("Safari bookmark file not found")
	}

	return bi.importFromFile(safariPath, "Safari")
}

// AutoImport detects and imports from all available browsers
func (bi *BrowserImporter) AutoImport() error {
	var importedFrom []string

	// Try Chrome
	if err := bi.ImportFromChrome(); err == nil {
		importedFrom = append(importedFrom, "Chrome")
	}

	// Try Firefox
	if err := bi.ImportFromFirefox(); err == nil {
		importedFrom = append(importedFrom, "Firefox")
	}

	// Try Safari
	if err := bi.ImportFromSafari(); err == nil {
		importedFrom = append(importedFrom, "Safari")
	}

	if len(importedFrom) == 0 {
		return fmt.Errorf("no browser bookmarks found")
	}

	fmt.Printf("Successfully imported from: %s\n", strings.Join(importedFrom, ", "))
	return nil
}

// SyncBookmarks syncs bookmarks and removes duplicates
func (bi *BrowserImporter) SyncBookmarks() error {
	ctx := context.Background()

	// Get last sync time
	lastSync, err := bi.redisClient.Get(ctx, RedisLastSyncKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}

	fmt.Println("Syncing bookmarks...")

	// Import from all browsers
	if err := bi.AutoImport(); err != nil {
		return err
	}

	// Clean duplicates
	if err := bi.CleanDuplicates(); err != nil {
		return err
	}

	// Update last sync time
	bi.redisClient.Set(ctx, RedisLastSyncKey, time.Now().Unix(), 0)

	fmt.Printf("Sync complete. Last sync: %s\n", lastSync)
	return nil
}

// importFromFile imports bookmarks from a specific file
func (bi *BrowserImporter) importFromFile(filePath, browser string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var bookmarks []BrowserBookmark

	// Parse based on browser type
	switch browser {
	case "Chrome":
		bookmarks = bi.parseChromeBookmarks(data)
	case "Firefox":
		bookmarks = bi.parseFirefoxBookmarks(data)
	case "Safari":
		bookmarks = bi.parseSafariBookmarks(data)
	default:
		return fmt.Errorf("unsupported browser: %s", browser)
	}

	if len(bookmarks) == 0 {
		return fmt.Errorf("no bookmarks found in %s", browser)
	}

	return bi.importBookmarks(bookmarks, browser)
}

// parseChromeBookmarks parses Chrome bookmark JSON
func (bi *BrowserImporter) parseChromeBookmarks(data []byte) []BrowserBookmark {
	var bookmarks []BrowserBookmark

	// Chrome bookmarks are in a nested structure
	roots := gjson.GetBytes(data, "roots")
	roots.ForEach(func(key, value gjson.Result) bool {
		bi.extractChromeBookmarks(value, "", &bookmarks)
		return true
	})

	return bookmarks
}

// extractChromeBookmarks recursively extracts bookmarks from Chrome structure
func (bi *BrowserImporter) extractChromeBookmarks(node gjson.Result, folder string, bookmarks *[]BrowserBookmark) {
	if node.Get("type").String() == "url" {
		// This is a bookmark
		bm := BrowserBookmark{
			URL:         node.Get("url").String(),
			Title:       node.Get("name").String(),
			Description: "",
			Tags:        []string{folder},
			CreatedAt:   node.Get("date_added").Int() / 1000000, // Chrome uses microseconds
			Folder:      folder,
		}

		if bm.URL != "" && bm.Title != "" {
			*bookmarks = append(*bookmarks, bm)
		}
	} else if node.Get("type").String() == "folder" {
		// This is a folder, recurse into children
		currentFolder := node.Get("name").String()
		if folder != "" {
			currentFolder = folder + "/" + currentFolder
		}

		children := node.Get("children")
		children.ForEach(func(key, value gjson.Result) bool {
			bi.extractChromeBookmarks(value, currentFolder, bookmarks)
			return true
		})
	}
}

// parseFirefoxBookmarks parses Firefox bookmark JSON
func (bi *BrowserImporter) parseFirefoxBookmarks(data []byte) []BrowserBookmark {
	var bookmarks []BrowserBookmark

	// Firefox bookmarks are in a different structure
	children := gjson.GetBytes(data, "children")
	children.ForEach(func(key, value gjson.Result) bool {
		bi.extractFirefoxBookmarks(value, "", &bookmarks)
		return true
	})

	return bookmarks
}

// extractFirefoxBookmarks recursively extracts bookmarks from Firefox structure
func (bi *BrowserImporter) extractFirefoxBookmarks(node gjson.Result, folder string, bookmarks *[]BrowserBookmark) {
	if node.Get("typeCode").Int() == 1 { // Bookmark
		bm := BrowserBookmark{
			URL:         node.Get("uri").String(),
			Title:       node.Get("title").String(),
			Description: "",
			Tags:        []string{folder},
			CreatedAt:   node.Get("dateAdded").Int() / 1000, // Firefox uses milliseconds
			Folder:      folder,
		}

		if bm.URL != "" && bm.Title != "" {
			*bookmarks = append(*bookmarks, bm)
		}
	} else if node.Get("typeCode").Int() == 2 { // Folder
		currentFolder := node.Get("title").String()
		if folder != "" {
			currentFolder = folder + "/" + currentFolder
		}

		children := node.Get("children")
		children.ForEach(func(key, value gjson.Result) bool {
			bi.extractFirefoxBookmarks(value, currentFolder, bookmarks)
			return true
		})
	}
}

// parseSafariBookmarks parses Safari bookmark plist (simplified)
func (bi *BrowserImporter) parseSafariBookmarks(data []byte) []BrowserBookmark {
	// Safari uses plist format, which is more complex to parse
	// For now, return empty - this would need a plist parser
	return []BrowserBookmark{}
}

// importBookmarks imports the parsed bookmarks into Redis
func (bi *BrowserImporter) importBookmarks(bookmarks []BrowserBookmark, browser string) error {
	ctx := context.Background()
	bar := progressbar.Default(int64(len(bookmarks)), fmt.Sprintf("Importing from %s", browser))

	imported := 0
	skipped := 0

	for _, bm := range bookmarks {
		bookmark := models.Bookmark{
			URL:         bm.URL,
			Title:       bm.Title,
			Description: bm.Description,
			Tags:        bm.Tags,
			CreatedAt:   bm.CreatedAt,
			UpdatedAt:   time.Now().Unix(),
			ID:          bi.generateID(bm.URL),
		}

		// Check for duplicates
		exists, err := bi.redisClient.SAdd(ctx, RedisURLSetKey, bookmark.URL).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			skipped++
			bar.Add(1)
			continue // Skip duplicates
		}

		// Add to search index
		jsonData, _ := json.Marshal(bookmark)
		if err := bi.redisClient.ZAdd(ctx, RedisBookmarksKey, &redis.Z{
			Score:  float64(bookmark.CreatedAt),
			Member: jsonData,
		}).Err(); err != nil {
			return err
		}

		// Index title terms
		terms := strings.Fields(strings.ToLower(bookmark.Title))
		for _, term := range terms {
			bi.redisClient.SAdd(ctx, RedisTitleSetKey, term)
		}

		imported++
		bar.Add(1)
	}

	bar.Finish()
	fmt.Printf("%s import complete: %d imported, %d skipped\n", browser, imported, skipped)
	return nil
}

// CleanDuplicates removes duplicate bookmarks
func (bi *BrowserImporter) CleanDuplicates() error {
	ctx := context.Background()

	// Get all bookmarks
	zRange := bi.redisClient.ZRangeWithScores(ctx, RedisBookmarksKey, 0, -1)
	results, err := zRange.Result()
	if err != nil {
		return err
	}

	urlMap := make(map[string]bool)
	var uniqueBookmarks []redis.Z

	for _, z := range results {
		var bm models.Bookmark
		if err := json.Unmarshal([]byte(z.Member.(string)), &bm); err != nil {
			continue
		}

		if !urlMap[bm.URL] {
			urlMap[bm.URL] = true
			uniqueBookmarks = append(uniqueBookmarks, z)
		}
	}

	// Clear and rebuild the bookmark index
	bi.redisClient.Del(ctx, RedisBookmarksKey)
	if len(uniqueBookmarks) > 0 {
		// Convert []redis.Z to []*redis.Z
		var zPointers []*redis.Z
		for i := range uniqueBookmarks {
			zPointers = append(zPointers, &uniqueBookmarks[i])
		}
		bi.redisClient.ZAdd(ctx, RedisBookmarksKey, zPointers...)
	}

	fmt.Printf("Removed %d duplicate bookmarks\n", len(results)-len(uniqueBookmarks))
	return nil
}

// getChromeBookmarkPath returns the Chrome bookmark file path
func (bi *BrowserImporter) getChromeBookmarkPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local", "Google", "Chrome", "User Data", "Default", "Bookmarks")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Google", "Chrome", "Default", "Bookmarks")
	case "linux":
		return filepath.Join(os.Getenv("HOME"), ".config", "google-chrome", "Default", "Bookmarks")
	default:
		return ""
	}
}

// getFirefoxBookmarkPath returns the Firefox bookmark file path
func (bi *BrowserImporter) getFirefoxBookmarkPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Mozilla", "Firefox", "Profiles", "bookmarks.json")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Firefox", "Profiles", "bookmarks.json")
	case "linux":
		return filepath.Join(os.Getenv("HOME"), ".mozilla", "firefox", "profiles", "bookmarks.json")
	default:
		return ""
	}
}

// getSafariBookmarkPath returns the Safari bookmark file path
func (bi *BrowserImporter) getSafariBookmarkPath() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Safari", "Bookmarks.plist")
	default:
		return ""
	}
}

// generateID generates a unique ID for a bookmark
func (bi *BrowserImporter) generateID(url string) string {
	return fmt.Sprintf("%x", len(url))
}
