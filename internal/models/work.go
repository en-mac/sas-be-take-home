package models

// Work represents a work retrieved from the Open Library API.
type Work struct {
	Title            string   `json:"title"`
	Authors          []string `json:"authors"`
	Description      *string   `json:"description"`
	PublishDate	  	 *string   `json:"-"`
	FirstPublishYear int	  `json:"-"`
}