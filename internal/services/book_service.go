// internal/services/book_service.go

package services

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "regexp"
    // "sort"
    "strconv"
    "strings"
    "sync"
    "time"

    "be-takehome-2024/internal/models"
)

// parseYear attempts to extract a four-digit year from a publish date string.
// Returns 0 if no valid year is found.
func parseYear(publishDate string) int {
    // Regular expression to find a four-digit year between 1000 and 2999
    re := regexp.MustCompile(`\b(1[0-9]{3}|2[0-9]{3})\b`)
    match := re.FindString(publishDate)
    if match == "" {
        return 0
    }
    year, err := strconv.Atoi(match)
    if err != nil {
        return 0
    }
    return year
}

// GetRecommendedBooks fetches books in the common subject and returns the top three recent books that are still in print.
func GetRecommendedBooks(ctx context.Context, subject string) ([]models.Work, error) {
    // Fetch books for the subject
    subjectURL := fmt.Sprintf("https://openlibrary.org/subjects/%s.json?limit=50&sort=new", strings.ReplaceAll(subject, " ", "_"))

    // Create HTTP request with context
    req, err := http.NewRequestWithContext(ctx, "GET", subjectURL, nil)
    if err != nil {
        return nil, fmt.Errorf("Error creating request for subject '%s': %v", subject, err)
    }

    // Perform the HTTP request
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("Error fetching books for subject '%s': %v", subject, err)
    }
    defer resp.Body.Close()

    // Read the response body
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("Error reading books response for subject '%s': %v", subject, err)
    }

    // Parse the JSON response
    var subjectResult struct {
        Works []struct {
            Title   string `json:"title"`
            Authors []struct {
                Name string `json:"name"`
                Key  string `json:"key"`
            } `json:"authors"`
            Key string `json:"key"`
        } `json:"works"`
    }

    if err := json.Unmarshal(body, &subjectResult); err != nil {
        return nil, fmt.Errorf("Error parsing books JSON for subject '%s': %v", subject, err)
    }

    // Prepare to process books concurrently
    var (
        recentBooks []models.Work
        mu          sync.Mutex
        wg          sync.WaitGroup
        concurrency = 6 // Limit the number of concurrent goroutines
        sem         = make(chan struct{}, concurrency)
        currentYear = time.Now().Year()
    )

    for _, work := range subjectResult.Works {
        wg.Add(1)
        sem <- struct{}{} // Acquire a semaphore slot

        // Capture variables for the goroutine
        work := work

        go func() {
            defer wg.Done()
            defer func() { <-sem }() // Release the semaphore slot

            // Fetch editions for the work
            workKey := strings.TrimPrefix(work.Key, "/works/")
            editionsURL := fmt.Sprintf("https://openlibrary.org/works/%s/editions.json?limit=50", workKey)

            // Create HTTP request with context
            editionsReq, err := http.NewRequestWithContext(ctx, "GET", editionsURL, nil)
            if err != nil {
                log.Printf("Error creating request for editions of work '%s': %v", work.Title, err)
                return
            }

            editionsResp, err := http.DefaultClient.Do(editionsReq)
            if err != nil {
                log.Printf("Error fetching editions for work '%s': %v", work.Title, err)
                return
            }
            defer editionsResp.Body.Close()

            editionsBody, err := ioutil.ReadAll(editionsResp.Body)
            if err != nil {
                log.Printf("Error reading editions for work '%s': %v", work.Title, err)
                return
            }

            // Parse editions
            var editionsResult struct {
                Entries []struct {
                    PublishDate string `json:"publish_date"`
                } `json:"entries"`
            }

            if err := json.Unmarshal(editionsBody, &editionsResult); err != nil {
                log.Printf("Error parsing editions JSON for work '%s': %v", work.Title, err)
                return
            }

            // Check if any edition was published within the last two years
            stillInPrint := false
            mostRecentYear := 0
            for _, edition := range editionsResult.Entries {
                year := parseYear(edition.PublishDate)
                if year >= currentYear-2 && year <= currentYear {
                    stillInPrint = true
                    if year > mostRecentYear {
                        mostRecentYear = year
                    }
                }
            }

            if !stillInPrint {
                return // Skip this work
            }

            // Fetch description
            var description *string
            descURL := fmt.Sprintf("https://openlibrary.org/works/%s.json", workKey)
            descReq, err := http.NewRequestWithContext(ctx, "GET", descURL, nil)
            if err != nil {
                log.Printf("Error creating request for description of work '%s': %v", work.Title, err)
                return
            }

            descResp, err := http.DefaultClient.Do(descReq)
            if err != nil {
                log.Printf("Error fetching description for work '%s': %v", work.Title, err)
            } else {
                defer descResp.Body.Close()
                descBody, err := ioutil.ReadAll(descResp.Body)
                if err != nil {
                    log.Printf("Error reading description for work '%s': %v", work.Title, err)
                } else {
                    var descResult struct {
                        Description interface{} `json:"description"`
                    }
                    if err := json.Unmarshal(descBody, &descResult); err != nil {
                        log.Printf("Error parsing description JSON for work '%s': %v", work.Title, err)
                    } else {
                        switch v := descResult.Description.(type) {
                        case string:
                            description = &v
                        case map[string]interface{}:
                            if val, ok := v["value"].(string); ok {
                                description = &val
                            }
                        }
                    }
                }
            }

            // Prepare authors list
            var authors []string
            for _, a := range work.Authors {
                authors = append(authors, a.Name)
            }

            // Create the Work struct
            recentWork := models.Work{
                Title:            work.Title,
                Authors:          authors,
                Description:      description,
                FirstPublishYear: mostRecentYear,
            }

            // Safely append to the recentBooks slice
            mu.Lock()
            recentBooks = append(recentBooks, recentWork)
            mu.Unlock()
        }()
    }

    // Wait for all goroutines to finish
    wg.Wait()

    if len(recentBooks) == 0 {
        return nil, fmt.Errorf("No recent books found for subject '%s'.", subject)
    }

    // // Sort by the most recent edition's publish year in descending order
    // sort.Slice(recentBooks, func(i, j int) bool {
    //     return recentBooks[i].FirstPublishYear > recentBooks[j].FirstPublishYear
    // })

    // Select the top three books
    if len(recentBooks) > 3 {
        recentBooks = recentBooks[:3]
    }

    return recentBooks, nil
}
