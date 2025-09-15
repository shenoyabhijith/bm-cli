package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Bookmark struct {
	URL   string
	Title string
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run bulk-import.go <bookmarks.html> <places.sqlite>")
		fmt.Println("Example: go run bulk-import.go ~/Downloads/bookmarks.html \"~/Library/Application Support/zen/Profiles/22t0dtwq.Default (release)/places.sqlite\"")
		return
	}

	htmlFile := os.Args[1]
	dbPath := os.Args[2]

	// Read and parse HTML bookmarks
	data, err := ioutil.ReadFile(htmlFile)
	if err != nil {
		log.Fatal("Failed to read bookmarks file:", err)
	}
	bookmarks := parseBookmarks(string(data))
	fmt.Printf("Found %d bookmarks\n", len(bookmarks))

	if len(bookmarks) == 0 {
		fmt.Println("No bookmarks found in HTML file")
		return
	}

	// Open Zen places.sqlite
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal("Failed to open DB:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Database is locked. Please close Zen browser first:", err)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	// Insert all bookmarks
	imported := 0
	skipped := 0
	for _, bm := range bookmarks {
		if insertBookmark(tx, bm.URL, bm.Title) {
			imported++
		} else {
			skipped++
		}
	}

	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Import complete: %d imported, %d skipped\n", imported, skipped)
}

// parseBookmarks extracts URLs and titles from <A HREF="...">title</A>
func parseBookmarks(html string) []Bookmark {
	re := regexp.MustCompile(`<A HREF="([^"]+)"[^>]*>([^<]+)</A>`)
	matches := re.FindAllStringSubmatch(html, -1)
	var bookmarks []Bookmark
	for _, m := range matches {
		url := strings.TrimSpace(m[1])
		title := strings.TrimSpace(m[2])
		
		// Skip invalid URLs
		if url == "" || title == "" || !strings.HasPrefix(url, "http") {
			continue
		}
		
		bookmarks = append(bookmarks, Bookmark{
			URL:   url,
			Title: title,
		})
	}
	return bookmarks
}

func insertBookmark(tx *sql.Tx, url, title string) bool {
	var placeID int64
	err := tx.QueryRow("SELECT id FROM moz_places WHERE url = ?", url).Scan(&placeID)

	if err == sql.ErrNoRows {
		// Insert into moz_places
		res, err := tx.Exec(`INSERT INTO moz_places 
			(url, title, rev_host, hidden, typed, frecency) 
			VALUES (?, ?, ?, 0, 0, -1)`,
			url, title, reverseHost(getHost(url)))
		if err != nil {
			log.Println("Insert moz_places failed:", err, url)
			return false
		}
		placeID, _ = res.LastInsertId()
	} else if err != nil {
		log.Println("Lookup moz_places failed:", err, url)
		return false
	} else {
		// URL already exists, skip
		return false
	}

	// Add to bookmarks (parent=2 → Bookmarks Menu)
	date := time.Now().UnixNano() / 1000
	_, err = tx.Exec(`INSERT INTO moz_bookmarks 
		(type, fk, parent, position, title, dateAdded, lastModified) 
		VALUES (1, ?, 2, 0, ?, ?, ?)`,
		placeID, title, date, date)
	if err != nil {
		log.Println("Insert moz_bookmarks failed:", err, url)
		return false
	}
	
	return true
}

// reverseHost converts "example.com" → "moc.elpmaxe."
func reverseHost(host string) string {
	if host == "" {
		return ""
	}
	runes := []rune(host + ".")
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// crude host extraction from URL
func getHost(url string) string {
	parts := strings.Split(strings.TrimPrefix(url, "https://"), "/")
	parts = strings.Split(strings.TrimPrefix(parts[0], "http://"), "/")
	return parts[0]
}
