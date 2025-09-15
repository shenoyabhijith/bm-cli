package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "test-zen.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT url, title FROM moz_places")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var url, title string
		if err := rows.Scan(&url, &title); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("URL: %s, Title: %s\n", url, title)
	}
}
