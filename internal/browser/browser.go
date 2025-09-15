package browser

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/abhijith/bookmark-cli/internal/models"
	"github.com/go-redis/redis/v8"
	_ "github.com/mattn/go-sqlite3"
	"github.com/schollz/progressbar/v3"
	"github.com/tidwall/gjson"
	"howett.net/plist"
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

	return bi.importFromSafariFile(safariPath)
}

// ImportFromZen imports bookmarks from Zen browser
func (bi *BrowserImporter) ImportFromZen() error {
	// Try to find exported HTML bookmarks first
	htmlPath := bi.getZenHTMLBookmarkPath()
	if htmlPath != "" {
		return bi.ImportFromHTMLFile(htmlPath)
	}

	// Fallback to direct database access (may be locked)
	zenPath := bi.getZenBookmarkPath()
	if zenPath == "" {
		return fmt.Errorf("Zen bookmark file not found. Please export bookmarks from Zen browser (Bookmarks > Import and Backup > Export Bookmarks to HTML) and save as 'bookmarks.html' in your Downloads folder")
	}

	return bi.importFromZenFile(zenPath)
}

// getZenHTMLBookmarkPath looks for exported HTML bookmark files
func (bi *BrowserImporter) getZenHTMLBookmarkPath() string {
	possiblePaths := []string{
		filepath.Join(os.Getenv("HOME"), "Downloads", "bookmarks.html"),
		filepath.Join(os.Getenv("HOME"), "Desktop", "bookmarks.html"),
		filepath.Join(os.Getenv("HOME"), "Documents", "bookmarks.html"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// importFromZenFile imports bookmarks from Zen SQLite database (fallback)
func (bi *BrowserImporter) importFromZenFile(filePath string) error {
	db, err := sql.Open("sqlite3", filePath+"?mode=ro&_timeout=1000")
	if err != nil {
		return fmt.Errorf("failed to open Zen database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("Zen database is locked (browser may be running). Please close Zen browser and try again: %v", err)
	}

	bookmarks := bi.parseZenBookmarks(db)
	if len(bookmarks) == 0 {
		return fmt.Errorf("no bookmarks found in Zen")
	}

	return bi.importBookmarks(bookmarks, "Zen")
}

// ImportFromArc imports bookmarks from Arc browser
func (bi *BrowserImporter) ImportFromArc() error {
	arcPath := bi.getArcBookmarkPath()
	if arcPath == "" {
		return fmt.Errorf("Arc bookmark file not found")
	}

	return bi.importFromFile(arcPath, "Arc")
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

	// Try Zen
	if err := bi.ImportFromZen(); err == nil {
		importedFrom = append(importedFrom, "Zen")
	}

	// Try Arc
	if err := bi.ImportFromArc(); err == nil {
		importedFrom = append(importedFrom, "Arc")
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

// importFromSafariFile imports bookmarks from Safari plist file
func (bi *BrowserImporter) importFromSafariFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("Safari bookmark access denied. Please grant Full Disk Access permission to Terminal in System Preferences > Security & Privacy > Privacy > Full Disk Access")
		}
		return fmt.Errorf("failed to read Safari bookmarks: %v", err)
	}

	var plistData interface{}
	_, err = plist.Unmarshal(data, &plistData)
	if err != nil {
		return fmt.Errorf("failed to parse Safari plist: %v", err)
	}

	bookmarks := bi.parseSafariBookmarks(plistData)
	if len(bookmarks) == 0 {
		return fmt.Errorf("no bookmarks found in Safari")
	}

	return bi.importBookmarks(bookmarks, "Safari")
}

// ImportFromHTMLFile imports bookmarks from HTML export file
func (bi *BrowserImporter) ImportFromHTMLFile(htmlFilePath string) error {
	data, err := os.ReadFile(htmlFilePath)
	if err != nil {
		return fmt.Errorf("failed to read HTML file: %v", err)
	}

	bookmarks := bi.parseHTMLBookmarks(data)
	if len(bookmarks) == 0 {
		return fmt.Errorf("no bookmarks found in HTML file")
	}

	fmt.Printf("Found %d bookmarks in HTML file\n", len(bookmarks))
	return bi.importBookmarks(bookmarks, "HTML Export")
}

// parseHTMLBookmarks parses HTML bookmark export format
func (bi *BrowserImporter) parseHTMLBookmarks(data []byte) []BrowserBookmark {
	var bookmarks []BrowserBookmark

	// Simple HTML parsing for bookmark files
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "<DT><A HREF=") {
			// Extract URL and title from HTML bookmark format
			urlStart := strings.Index(line, "HREF=\"") + 6
			urlEnd := strings.Index(line[urlStart:], "\"")
			if urlEnd == -1 {
				continue
			}
			url := line[urlStart : urlStart+urlEnd]

			titleStart := strings.Index(line, ">") + 1
			titleEnd := strings.Index(line[titleStart:], "</A>")
			if titleEnd == -1 {
				continue
			}
			title := line[titleStart : titleStart+titleEnd]

			bm := BrowserBookmark{
				URL:         url,
				Title:       title,
				Description: "",
				Tags:        []string{"Imported"},
				CreatedAt:   time.Now().Unix(),
				Folder:      "Imported",
			}

			bookmarks = append(bookmarks, bm)
		}
	}

	return bookmarks
}

// parseSafariBookmarks parses Safari bookmark plist
func (bi *BrowserImporter) parseSafariBookmarks(plistData interface{}) []BrowserBookmark {
	var bookmarks []BrowserBookmark

	// Safari plist structure is complex, this is a simplified parser
	if plistMap, ok := plistData.(map[string]interface{}); ok {
		if children, exists := plistMap["Children"]; exists {
			bi.extractSafariBookmarks(children, "", &bookmarks)
		}
	}

	return bookmarks
}

// extractSafariBookmarks recursively extracts bookmarks from Safari plist
func (bi *BrowserImporter) extractSafariBookmarks(node interface{}, folder string, bookmarks *[]BrowserBookmark) {
	if nodeArray, ok := node.([]interface{}); ok {
		for _, item := range nodeArray {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, exists := itemMap["WebBookmarkType"]; exists {
					if itemType == "WebBookmarkTypeLeaf" {
						// This is a bookmark
						if urlData, exists := itemMap["URLString"]; exists {
							if titleData, exists := itemMap["URIDictionary"]; exists {
								if titleMap, ok := titleData.(map[string]interface{}); ok {
									if title, exists := titleMap["title"]; exists {
										bm := BrowserBookmark{
											URL:         fmt.Sprintf("%v", urlData),
											Title:       fmt.Sprintf("%v", title),
											Description: "",
											Tags:        []string{folder},
											CreatedAt:   time.Now().Unix(),
											Folder:      folder,
										}
										*bookmarks = append(*bookmarks, bm)
									}
								}
							}
						}
					} else if itemType == "WebBookmarkTypeList" {
						// This is a folder
						if titleData, exists := itemMap["Title"]; exists {
							currentFolder := fmt.Sprintf("%v", titleData)
							if folder != "" {
								currentFolder = folder + "/" + currentFolder
							}
							if children, exists := itemMap["Children"]; exists {
								bi.extractSafariBookmarks(children, currentFolder, bookmarks)
							}
						}
					}
				}
			}
		}
	}
}

// parseZenBookmarks parses Zen SQLite database
func (bi *BrowserImporter) parseZenBookmarks(db *sql.DB) []BrowserBookmark {
	var bookmarks []BrowserBookmark

	// First, let's check if the tables exist
	tablesQuery := "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%bookmark%'"
	rows, err := db.Query(tablesQuery)
	if err != nil {
		fmt.Printf("Error checking tables: %v\n", err)
		return bookmarks
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err == nil {
			tables = append(tables, tableName)
		}
	}
	fmt.Printf("Found bookmark tables: %v\n", tables)

	// Try the standard Firefox query
	query := `
		SELECT b.title, p.url, b.dateAdded, f.title as folder
		FROM moz_bookmarks b
		JOIN moz_places p ON b.fk = p.id
		LEFT JOIN moz_bookmarks f ON b.parent = f.id
		WHERE b.type = 1 AND p.url IS NOT NULL
	`

	rows, err = db.Query(query)
	if err != nil {
		fmt.Printf("Error executing query: %v\n", err)
		return bookmarks
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var title, url, folder string
		var dateAdded int64

		err := rows.Scan(&title, &url, &dateAdded, &folder)
		if err != nil {
			fmt.Printf("Error scanning row: %v\n", err)
			continue
		}

		bm := BrowserBookmark{
			URL:         url,
			Title:       title,
			Description: "",
			Tags:        []string{folder},
			CreatedAt:   dateAdded / 1000000, // Convert microseconds to seconds
			Folder:      folder,
		}

		bookmarks = append(bookmarks, bm)
		count++
	}

	fmt.Printf("Found %d bookmarks in Zen database\n", count)
	return bookmarks
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

// getZenBookmarkPath returns the Zen bookmark database path
func (bi *BrowserImporter) getZenBookmarkPath() string {
	switch runtime.GOOS {
	case "darwin":
		// Zen stores bookmarks in places.sqlite in the profiles directory
		zenProfilesDir := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "zen", "Profiles")
		if profiles, err := os.ReadDir(zenProfilesDir); err == nil {
			for _, profile := range profiles {
				if profile.IsDir() {
					placesPath := filepath.Join(zenProfilesDir, profile.Name(), "places.sqlite")
					if _, err := os.Stat(placesPath); err == nil {
						return placesPath
					}
				}
			}
		}
		return ""
	case "linux":
		// Linux: ~/.zen/profiles/
		zenProfilesDir := filepath.Join(os.Getenv("HOME"), ".zen", "profiles")
		if profiles, err := os.ReadDir(zenProfilesDir); err == nil {
			for _, profile := range profiles {
				if profile.IsDir() {
					placesPath := filepath.Join(zenProfilesDir, profile.Name(), "places.sqlite")
					if _, err := os.Stat(placesPath); err == nil {
						return placesPath
					}
				}
			}
		}
		return ""
	case "windows":
		// Windows: %APPDATA%\zen\profiles\
		zenProfilesDir := filepath.Join(os.Getenv("APPDATA"), "zen", "profiles")
		if profiles, err := os.ReadDir(zenProfilesDir); err == nil {
			for _, profile := range profiles {
				if profile.IsDir() {
					placesPath := filepath.Join(zenProfilesDir, profile.Name(), "places.sqlite")
					if _, err := os.Stat(placesPath); err == nil {
						return placesPath
					}
				}
			}
		}
		return ""
	default:
		return ""
	}
}

// getArcBookmarkPath returns the Arc bookmark file path
func (bi *BrowserImporter) getArcBookmarkPath() string {
	switch runtime.GOOS {
	case "darwin":
		// Arc stores bookmarks in Bookmarks JSON file inside each profile
		// macOS: ~/Library/Application Support/Arc/User Data/
		arcUserDataDir := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Arc", "User Data")

		// Check Default profile first
		defaultPath := filepath.Join(arcUserDataDir, "Default", "Bookmarks")
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath
		}

		// Check other profiles (Profile 1, Profile 2, etc.)
		if profiles, err := os.ReadDir(arcUserDataDir); err == nil {
			for _, profile := range profiles {
				if profile.IsDir() && strings.HasPrefix(profile.Name(), "Profile") {
					profilePath := filepath.Join(arcUserDataDir, profile.Name(), "Bookmarks")
					if _, err := os.Stat(profilePath); err == nil {
						return profilePath
					}
				}
			}
		}
		return ""
	case "linux":
		// Linux: ~/.config/Arc/User Data/
		arcUserDataDir := filepath.Join(os.Getenv("HOME"), ".config", "Arc", "User Data")
		return filepath.Join(arcUserDataDir, "Default", "Bookmarks")
	case "windows":
		// Windows: %LOCALAPPDATA%\Arc\User Data\
		arcUserDataDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Arc", "User Data")
		return filepath.Join(arcUserDataDir, "Default", "Bookmarks")
	default:
		return ""
	}
}

// generateID generates a unique ID for a bookmark
func (bi *BrowserImporter) generateID(url string) string {
	return fmt.Sprintf("%x", len(url))
}
