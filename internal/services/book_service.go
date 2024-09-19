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
    "sort"
    "strconv"
    "strings"
    "sync"
    "time"

    "be-takehome-2024/internal/models"
)

type bookJob struct {
    Title            string
    Authors          []string
    FirstPublishYear int
    PublishDate      string
    Key              string
}

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

// GetRecommendedBooks fetches books in the common subject and returns the top three recent books published within the last two years.
func GetRecommendedBooks(ctx context.Context, subject string) ([]models.Work, error) {
    // Fetch books for the subject with context, enforcing English language
    subjectURL := fmt.Sprintf("https://openlibrary.org/subjects/%s.json?limit=50&sort=new", strings.ReplaceAll(subject, " ", "_"))
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
            Title            string `json:"title"`
            Authors          []struct {
                Name string `json:"name"`
                Key  string `json:"key"`
            } `json:"authors"`
            EditionCount     int    `json:"edition_count"`
            Key              string `json:"key"`
            CoverID          int    `json:"cover_id"`
            FirstPublishYear int    `json:"first_publish_year"`
            PublishDate      string `json:"publish_date"` // Added PublishDate
        } `json:"works"`
    }

    if err := json.Unmarshal(body, &subjectResult); err != nil {
        return nil, fmt.Errorf("Error parsing books JSON for subject '%s': %v", subject, err)
    }

    // Define channels for jobs and results
    jobs := make(chan bookJob, len(subjectResult.Works))
    results := make(chan models.Work, len(subjectResult.Works))
    errCh := make(chan error, len(subjectResult.Works))

    // Define number of workers
    numWorkers := 10
    wg := sync.WaitGroup{}

    // Start worker goroutines
    for w := 1; w <= numWorkers; w++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            for job := range jobs {
                // Fetch description
                description := ""
                workKey := strings.TrimPrefix(job.Key, "/works/")
                descURL := fmt.Sprintf("https://openlibrary.org/works/%s.json", workKey)

                // Create HTTP request with context
                descReq, err := http.NewRequestWithContext(ctx, "GET", descURL, nil)
                if err != nil {
                    log.Printf("Worker %d: Error creating request for work '%s': %v", workerID, job.Title, err)
                    errCh <- fmt.Errorf("Work '%s': %v", job.Title, err)
                    continue
                }

                // Perform the HTTP request
                descResp, err := http.DefaultClient.Do(descReq)
                if err != nil {
                    log.Printf("Worker %d: Error fetching description for work '%s': %v", workerID, job.Title, err)
                    errCh <- fmt.Errorf("Work '%s': %v", job.Title, err)
                    continue
                }

                // Read and parse the description
                func() {
                    defer descResp.Body.Close()

                    // Read the response body
                    descBody, err := ioutil.ReadAll(descResp.Body)
                    if err != nil {
                        log.Printf("Worker %d: Error reading description for work '%s': %v", workerID, job.Title, err)
                        errCh <- fmt.Errorf("Work '%s': %v", job.Title, err)
                        return
                    }

                    // Parse the JSON response
                    var descResult struct {
                        Description interface{} `json:"description"`
                    }
                    if err := json.Unmarshal(descBody, &descResult); err != nil {
                        log.Printf("Worker %d: Error parsing description JSON for work '%s': %v", workerID, job.Title, err)
                        errCh <- fmt.Errorf("Work '%s': %v", job.Title, err)
                        return
                    }

                    // Extract description based on type
                    switch v := descResult.Description.(type) {
                    case string:
                        description = v
                    case map[string]interface{}:
                        if val, ok := v["value"].(string); ok {
                            description = val
                        }
                    }
                }()

                // Create the Work struct
                work := models.Work{
                    Title:            job.Title,
                    Authors:          job.Authors,
                    Description:      nil, // Default to nil
                    FirstPublishYear: job.FirstPublishYear,
                }

                if description != "" {
                    work.Description = &description
                }

                // Send the work to the results channel
                select {
                case results <- work:
                case <-ctx.Done():
                    return
                }
            }
        }(w)
    }

    // Enqueue jobs with added validation
    currentYear := time.Now().Year()
    for _, book := range subjectResult.Works {
        // Parse the publish year from publish_date if available
        publishYear := book.FirstPublishYear
        if book.PublishDate != "" {
            parsedYear := parseYear(book.PublishDate)
            if parsedYear != 0 {
                publishYear = parsedYear
            }
        }

        // Validation: Exclude books with future publish years
        if publishYear == 0 || publishYear < currentYear-2 || publishYear > currentYear {
            if publishYear > currentYear {
                log.Printf("Excluded book '%s' with future publish year: %d", book.Title, publishYear)
            }
            continue // Skip this book
        }

        // Collect authors' names
        var authors []string
        for _, a := range book.Authors {
            authors = append(authors, a.Name)
        }

        jobs <- bookJob{
            Title:            book.Title,
            Authors:          authors,
            FirstPublishYear: book.FirstPublishYear,
            PublishDate:      book.PublishDate,
            Key:              book.Key,
        }
    }
    close(jobs) // No more jobs

    // Wait for all workers to finish
    go func() {
        wg.Wait()
        close(results)
        close(errCh)
    }()

    // Collect results
    var recentBooks []models.Work
    for work := range results {
        recentBooks = append(recentBooks, work)
    }

    // Handle errors if necessary
    for err := range errCh {
        log.Printf("Error fetching book description: %v", err)
    }

    if len(recentBooks) == 0 {
        return nil, fmt.Errorf("No recent books found for subject '%s'.", subject)
    }

    // Sort by FirstPublishYear descending (most recent first)
    sort.Slice(recentBooks, func(i, j int) bool {
        return recentBooks[i].FirstPublishYear > recentBooks[j].FirstPublishYear
    })

    // Select the top three books
    if len(recentBooks) > 3 {
        recentBooks = recentBooks[:3]
    }

    return recentBooks, nil
}
