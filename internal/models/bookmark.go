package models

type Bookmark struct {
	URL         string   `json:"url" redis:"url"`
	Title       string   `json:"title" redis:"title"`
	Description string   `json:"description" redis:"description"`
	Tags        []string `json:"tags" redis:"tags"`
	CreatedAt   int64    `json:"created_at" redis:"created_at"`
	UpdatedAt   int64    `json:"updated_at" redis:"updated_at"`
	ID          string   `json:"id" redis:"id"`
}



