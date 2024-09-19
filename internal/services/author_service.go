package services

import (
	"context"
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
func ResolveAuthorKeys(ctx context.Context, authors []string) ([]models.Author, error) {
	var (
		authorKeys []models.Author
		mu         sync.Mutex
		wg         sync.WaitGroup
	)

	// Define concurrency limit
	concurrency := 20
	sem := make(chan struct{}, concurrency)

	// Channel to collect errors from goroutines
	errCh := make(chan error, len(authors))

	for _, authorName := range authors {
		wg.Add(1)
		sem <- struct{}{} // Acquire a semaphore slot

		// Capture authorName
		authorName := authorName

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release the semaphore slot

			// Replace spaces with '+' for URL encoding
			queryName := strings.ReplaceAll(authorName, " ", "+")

			// Open Library Search API URL
			searchURL := fmt.Sprintf("https://openlibrary.org/search/authors.json?q=%s", queryName)

			// Create HTTP request with context
			req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
			if err != nil {
				log.Printf("Error creating request for author '%s': %v", authorName, err)
				errCh <- fmt.Errorf("Author '%s': %v", authorName, err)
				return
			}

			// Perform the HTTP request
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("Error fetching data for author '%s': %v", authorName, err)
				errCh <- fmt.Errorf("Author '%s': %v", authorName, err)
				return
			}
			defer resp.Body.Close()

			// Check the status code
			if resp.StatusCode != http.StatusOK {
				bodySnippet, _ := ioutil.ReadAll(resp.Body)
				log.Printf("Non-OK HTTP status for author '%s': %s - %s", authorName, resp.Status, string(bodySnippet))
				errCh <- fmt.Errorf("Author '%s': received status %s", authorName, resp.Status)
				return
			}

			// Read the response body
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error reading response for author '%s': %v", authorName, err)
				errCh <- fmt.Errorf("Author '%s': %v", authorName, err)
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
				errCh <- fmt.Errorf("Author '%s': %v", authorName, err)
				return
			}

			// No authors found
			if len(result.Docs) == 0 {
				log.Printf("No authors found for '%s'.", authorName)
				errCh <- fmt.Errorf("No authors found for '%s'", authorName)
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

			// Append to the slice safely
			mu.Lock()
			authorKeys = append(authorKeys, selectedAuthor)
			mu.Unlock()
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errCh)

	// Check for errors
	if len(errCh) > 0 {
		errMessages := []string{}
		for err := range errCh {
			errMessages = append(errMessages, err.Error())
		}
		return nil, fmt.Errorf(strings.Join(errMessages, "; "))
	}

	return authorKeys, nil
}
