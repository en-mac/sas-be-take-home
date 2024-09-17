package models

// Work represents a work retrieved from the Open Library API.
type Work struct {
	Title            string   `json:"title"`
	Authors          []Author `json:"authors"`
	Description      string   `json:"description"`
	FirstPublishYear int      `json:"first_publish_year"`
}