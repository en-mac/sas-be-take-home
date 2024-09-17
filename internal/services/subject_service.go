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

    "be-takehome-2024/internal/models"
)

// GetSubjectAuthorCounts retrieves subjects per author and counts how many authors have written in each subject concurrently.
func GetSubjectAuthorCounts(authors []models.Author) (map[string]int, error) {
	subjectAuthorCount := make(map[string]int)
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		concurrency = 200 // Limit the number of concurrent goroutines
		sem         = make(chan struct{}, concurrency)
	)

	for _, author := range authors {
		wg.Add(1)
		sem <- struct{}{} // Acquire a semaphore slot

		go func(author models.Author) {
			defer wg.Done()
			defer func() { <-sem }() // Release the semaphore slot

			// Fetch works for the author
			worksURL := fmt.Sprintf("https://openlibrary.org/authors/%s/works.json?limit=100", author.Key)

			resp, err := http.Get(worksURL)
			if err != nil {
				log.Printf("Error fetching works for author '%s': %v", author.Name, err)
				return
			}
			// Ensure the response body is closed promptly
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("Error reading works response for author '%s': %v", author.Name, err)
				return
			}

			// Parse the JSON response
			var worksResult struct {
				Entries []struct {
					Title    string   `json:"title"`
					Subjects []string `json:"subjects"`
				} `json:"entries"`
			}

			if err := json.Unmarshal(body, &worksResult); err != nil {
				log.Printf("Error parsing works JSON for author '%s': %v", author.Name, err)
				return
			}

			// Collect unique subjects for the author
			subjectsSet := make(map[string]struct{})
			for _, work := range worksResult.Entries {
				for _, subject := range work.Subjects {
					normalizedSubject := strings.ToLower(strings.TrimSpace(subject))
					subjectsSet[normalizedSubject] = struct{}{}
				}
			}

			// Safely update the subjectAuthorCount map
			mu.Lock()
			for subject := range subjectsSet {
				subjectAuthorCount[subject]++
			}
			mu.Unlock()
		}(author)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return subjectAuthorCount, nil
}

// FindMostCommonSubject identifies common subjects between two users and selects the most prominent one.
func FindMostCommonSubject(user1Subjects, user2Subjects map[string]int) (string, error) {
	commonSubjects := make(map[string]int)

	// Find common subjects and sum the number of authors
	for subject, count1 := range user1Subjects {
		if count2, exists := user2Subjects[subject]; exists {
			commonSubjects[subject] = count1 + count2
		}
	}

	if len(commonSubjects) == 0 {
		return "", fmt.Errorf("No common subjects found between the users")
	}

	// Sort the common subjects by total author count in descending order
	type subjectCount struct {
		Subject string
		Count   int
	}
	var subjectCounts []subjectCount
	for subject, count := range commonSubjects {
		subjectCounts = append(subjectCounts, subjectCount{Subject: subject, Count: count})
	}

	sort.Slice(subjectCounts, func(i, j int) bool {
		return subjectCounts[i].Count > subjectCounts[j].Count
	})

	// Return the most prominent common subject
	return subjectCounts[0].Subject, nil
}