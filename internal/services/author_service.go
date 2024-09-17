package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"

    "be-takehome-2024/internal/models"
)

// ResolveAuthorKeys searches for authors and returns their Open Library keys concurrently.
func ResolveAuthorKeys(authors []string) ([]models.Author, error) {
	var (
		authorKeys   []models.Author
		wg           sync.WaitGroup
		mu           sync.Mutex
		concurrency  = 10 // Limit the number of concurrent goroutines
		sem          = make(chan struct{}, concurrency)
	)

	for _, authorName := range authors {
		wg.Add(1)
		sem <- struct{}{} // Acquire a semaphore slot

		// Capture authorName to avoid issues with goroutine closure
		authorName := authorName

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release the semaphore slot

			// Replace spaces with '+' for URL encoding
			queryName := strings.ReplaceAll(authorName, " ", "+")

			// Open Library Search API URL
			searchURL := fmt.Sprintf("https://openlibrary.org/search/authors.json?q=%s", queryName)

			// Make the API request
			resp, err := http.Get(searchURL)
			if err != nil {
				log.Printf("Error fetching data for author '%s': %v", authorName, err)
				return
			}
			// Ensure the response body is closed promptly
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("Error reading response for author '%s': %v", authorName, err)
				return
			}

			// Parse the JSON response
			var result struct {
				Docs []struct {
					Name      string `json:"name"`
					Key       string `json:"key"`
					WorkCount int    `json:"work_count"`
				} `json:"docs"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				log.Printf("Error parsing JSON for author '%s': %v", authorName, err)
				return
			}

			// No authors found
			if len(result.Docs) == 0 {
				log.Printf("No authors found for '%s'.", authorName)
				return
			}

			// Select the author with the highest work_count
			var selectedAuthor models.Author
			maxWorkCount := -1
			for _, doc := range result.Docs {
				if doc.WorkCount > maxWorkCount {
					maxWorkCount = doc.WorkCount
					// Ensure the key does not include leading slashes
					key := strings.TrimPrefix(doc.Key, "/authors/")
					selectedAuthor = models.Author{
						Name:      doc.Name,
						Key:       key,
						WorkCount: doc.WorkCount,
					}
				}
			}

			// Safely append to the authorKeys slice
			mu.Lock()
			authorKeys = append(authorKeys, selectedAuthor)
			log.Printf("Author '%s' retrieved with WorkCount: %d", selectedAuthor.Name, selectedAuthor.WorkCount)

			mu.Unlock()
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return authorKeys, nil
}