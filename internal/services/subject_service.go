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

// SubjectAuthorResult holds both aggregate subject counts and per-author subjects.
type SubjectAuthorResult struct {
	Aggregate  map[string]int      // Aggregate subject counts across all authors
	PerAuthor  map[string][]string // Subjects per individual author
	ProcessedW map[string]struct{} // Set of processed work IDs
}

// GetSubjectAuthorCounts retrieves subjects per author and counts how many authors have written in each subject concurrently.
// It ensures that each work is processed only once using work IDs.
func GetSubjectAuthorCounts(ctx context.Context, authors []models.Author) (SubjectAuthorResult, error) {
	subjectAuthorCount := make(map[string]int)
	perAuthorSubjects := make(map[string][]string)
	processedWorks := make(map[string]struct{}) // To track processed work IDs

	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		concurrency = 20 // Limit the number of concurrent goroutines
		sem         = make(chan struct{}, concurrency)
	)

	// Channel to collect errors from goroutines
	errCh := make(chan error, len(authors))

	for _, author := range authors {
		wg.Add(1)
		sem <- struct{}{} // Acquire a semaphore slot

		// Capture the current author to avoid closure issues
		author := author

		go func(author models.Author) {
			defer wg.Done()
			defer func() { <-sem }() // Release the semaphore slot

			// Fetch works for the author with context
			worksURL := fmt.Sprintf("https://openlibrary.org/authors/%s/works.json?limit=100", author.Key)

			// Create HTTP request with context
			req, err := http.NewRequestWithContext(ctx, "GET", worksURL, nil)
			if err != nil {
				log.Printf("Error creating request for author '%s': %v", author.Name, err)
				errCh <- fmt.Errorf("Author '%s': %v", author.Name, err)
				return
			}

			// Perform the HTTP request
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("Error fetching works for author '%s': %v", author.Name, err)
				errCh <- fmt.Errorf("Author '%s': %v", author.Name, err)
				return
			}
			defer resp.Body.Close()

			// Read the response body
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error reading works response for author '%s': %v", author.Name, err)
				errCh <- fmt.Errorf("Author '%s': %v", author.Name, err)
				return
			}

			// Parse the JSON response
			var worksResult struct {
				Entries []struct {
					Title    string   `json:"title"`
					Subjects []string `json:"subjects"`
					Key      string   `json:"key"` // Work ID
				} `json:"entries"`
			}
			if err := json.Unmarshal(body, &worksResult); err != nil {
				log.Printf("Error parsing works JSON for author '%s': %v", author.Name, err)
				errCh <- fmt.Errorf("Author '%s': %v", author.Name, err)
				return
			}

			// Collect unique subjects for the author, ensuring unique works
			subjectsSet := make(map[string]struct{})
			for i, work := range worksResult.Entries {
				// Log the author's name and the work number
				log.Printf("Author: %s, Work %d: %s, Subject: %s", author.Name, i+1, work.Title, work.Subjects)

				for _, subject := range work.Subjects {
					normalizedSubject := strings.ToLower(strings.TrimSpace(subject))
					subjectsSet[normalizedSubject] = struct{}{}
				}
			}

			// Safely update the aggregate and per-author subject counts
			mu.Lock()
			for subject := range subjectsSet {
				subjectAuthorCount[subject]++
				perAuthorSubjects[author.Name] = append(perAuthorSubjects[author.Name], subject)
			}
			mu.Unlock()
		}(author)
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
		return SubjectAuthorResult{}, fmt.Errorf(strings.Join(errMessages, "; "))
	}

	return SubjectAuthorResult{
		Aggregate:  subjectAuthorCount,
		PerAuthor:  perAuthorSubjects,
		ProcessedW: processedWorks,
	}, nil
}

func FindMostCommonSubject(user1Subjects, user2Subjects map[string]int) (string, error) {
    var (
        mostCommonSubject string
        highestCount      int
    )

    for subject, count1 := range user1Subjects {
        if count2, exists := user2Subjects[subject]; exists {
            totalCount := count1 + count2
            if totalCount > highestCount {
                highestCount = totalCount
                mostCommonSubject = subject
            }
        }
    }

    if mostCommonSubject == "" {
        return "", fmt.Errorf("No common subjects found between the users")
    }

    return mostCommonSubject, nil
}
