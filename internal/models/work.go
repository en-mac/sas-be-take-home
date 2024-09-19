package models

type Work struct {
	Title            string   `json:"title"`
	Authors          []string `json:"authors"`
	Description      *string   `json:"description"`
}