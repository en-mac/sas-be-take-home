package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"be-takehome-2024/internal/models"
)

// GetRecommendedBooks fetches books in the common subject and returns the top three recent books with descriptions fetched concurrently.
func GetRecommendedBooks(subject string) ([]models.Work, error) {
	// Fetch books for the subject
	subjectURL := fmt.Sprintf("https://openlibrary.org/subjects/%s.json?limit=50&sort=new", strings.ReplaceAll(subject, " ", "_"))

	resp, err := http.Get(subjectURL)
	if err != nil {
		return nil, fmt.Errorf("Error fetching books for subject '%s': %v", subject, err)
	}
	// Ensure the response body is closed promptly
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("Error reading books response for subject '%s': %v", subject, err)
	}

	// Parse the JSON response
	var subjectResult struct {
		Works []struct {
			Title            string   `json:"title"`
			Authors          []struct {
				Name string `json:"name"`
				Key  string `json:"key"`
			} `json:"authors"`
			EditionCount     int      `json:"edition_count"`
			FirstPublishYear int      `json:"first_publish_year"`
			Key              string   `json:"key"`
			CoverID          int      `json:"cover_id"`
			PublishDate      []string `json:"publish_date"`
		} `json:"works"`
	}

	if err := json.Unmarshal(body, &subjectResult); err != nil {
		return nil, fmt.Errorf("Error parsing books JSON for subject '%s': %v", subject, err)
	}

	// Filter books published in the last 2 years and not in the future
	currentDate := time.Now()
	twoYearsAgo := currentDate.AddDate(-2, 0, 0)

	var (
		recentBooks    []models.Work
		mu             sync.Mutex
		wg             sync.WaitGroup
		concurrency    = 200 // Limit the number of concurrent goroutines for fetching descriptions
		sem            = make(chan struct{}, concurrency)
		booksToProcess []struct {
			Work struct {
				Title            string   `json:"title"`
				Authors          []struct {
					Name string `json:"name"`
					Key  string `json:"key"`
				} `json:"authors"`
				EditionCount     int      `json:"edition_count"`
				FirstPublishYear int      `json:"first_publish_year"`
				Key              string   `json:"key"`
				CoverID          int      `json:"cover_id"`
				PublishDate      []string `json:"publish_date"`
			}
		}
	)

	// Pre-filter works based on publication date
	for _, work := range subjectResult.Works {
		var publishDate time.Time
		// Try to parse the most recent publish date
		for _, dateStr := range work.PublishDate {
			parsedDate, err := time.Parse("2006", dateStr)
			if err != nil {
				parsedDate, err = time.Parse("January 2, 2006", dateStr)
			}
			if err != nil {
				parsedDate, err = time.Parse("2006-01-02", dateStr)
			}
			if err == nil {
				if publishDate.IsZero() || parsedDate.After(publishDate) {
					publishDate = parsedDate
				}
			}
		}

		// If publishDate is zero, fall back to FirstPublishYear
		if publishDate.IsZero() && work.FirstPublishYear != 0 {
			publishDate = time.Date(work.FirstPublishYear, 1, 1, 0, 0, 0, 0, time.UTC)
		}

		if publishDate.IsZero() {
			continue
		}

		// Exclude books published in the future
		if publishDate.After(currentDate) {
			continue
		}

		// Include books published in the last 2 years
		if publishDate.After(twoYearsAgo) || publishDate.Equal(twoYearsAgo) {
			booksToProcess = append(booksToProcess, struct {
				Work struct {
					Title            string   `json:"title"`
					Authors          []struct {
						Name string `json:"name"`
						Key  string `json:"key"`
					} `json:"authors"`
					EditionCount     int      `json:"edition_count"`
					FirstPublishYear int      `json:"first_publish_year"`
					Key              string   `json:"key"`
					CoverID          int      `json:"cover_id"`
					PublishDate      []string `json:"publish_date"`
				}
			}{Work: work})
		}
	}

	// Process each book to fetch its description concurrently
	for _, book := range booksToProcess {
		wg.Add(1)
		sem <- struct{}{} // Acquire a semaphore slot

		book := book // Capture the current book

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release the semaphore slot

			// Prepare authors list
			var authors []models.Author
			for _, a := range book.Work.Authors {
				authors = append(authors, models.Author{
					Name: a.Name,
					Key:  a.Key,
				})
			}

			// Fetch description if available
			description := ""
			workKey := strings.TrimPrefix(book.Work.Key, "/works/")
			descURL := fmt.Sprintf("https://openlibrary.org/works/%s.json", workKey)
			descResp, err := http.Get(descURL)
			if err != nil {
				log.Printf("Error fetching description for work '%s': %v", book.Work.Title, err)
			} else {
				// Ensure the response body is closed promptly
				descBody, err := ioutil.ReadAll(descResp.Body)
				descResp.Body.Close()
				if err != nil {
					log.Printf("Error reading description for work '%s': %v", book.Work.Title, err)
				} else {
					var descResult struct {
						Description interface{} `json:"description"`
					}
					if err := json.Unmarshal(descBody, &descResult); err != nil {
						log.Printf("Error parsing description JSON for work '%s': %v", book.Work.Title, err)
					} else {
						switch v := descResult.Description.(type) {
						case string:
							description = v
						case map[string]interface{}:
							if val, ok := v["value"].(string); ok {
								description = val
							}
						}
					}
				}
			}

			// Create the Work struct
			recentBook := models.Work{
				Title:            book.Work.Title,
				Authors:          authors,
				Description:      description,
				FirstPublishYear: book.Work.FirstPublishYear,
			}

			// Safely append to the recentBooks slice
			mu.Lock()
			recentBooks = append(recentBooks, recentBook)
			mu.Unlock()
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()

	if len(recentBooks) == 0 {
		return nil, fmt.Errorf("No recent books found for subject '%s'.", subject)
	}

	// Sort the books by publication date in descending order
	sort.Slice(recentBooks, func(i, j int) bool {
		return recentBooks[i].FirstPublishYear > recentBooks[j].FirstPublishYear
	})

	// Select the top three books
	if len(recentBooks) > 3 {
		recentBooks = recentBooks[:3]
	}

	return recentBooks, nil
}