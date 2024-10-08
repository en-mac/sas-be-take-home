package services

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "strings"
    "time"
    "be-takehome-2024/internal/models"
)

// GetRecommendedBooks fetches books in the common subject and returns the top three recent books.
func GetRecommendedBooks(ctx context.Context, subject string) ([]models.Work, error) {
    // Fetch books for the subject
    subjectURL := fmt.Sprintf("https://openlibrary.org/subjects/%s.json?limit=50&sort=new", strings.ReplaceAll(subject, " ", "_"))

    req, err := http.NewRequestWithContext(ctx, "GET", subjectURL, nil)
    if err != nil {
        return nil, fmt.Errorf("error creating request for subject '%s': %v", subject, err)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("error fetching books for subject '%s': %v", subject, err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("error reading books response for subject '%s': %v", subject, err)
    }

    var subjectResult struct {
        Works []struct {
            Title          string `json:"title"`
            Authors        []struct {
                Name string `json:"name"`
            } `json:"authors"`
            Key            string `json:"key"`
            FirstPublishYear int   `json:"first_publish_year"` // Ensure this field is returned by API
        } `json:"works"`
    }

    if err := json.Unmarshal(body, &subjectResult); err != nil {
        return nil, fmt.Errorf("error parsing books JSON for subject '%s': %v", subject, err)
    }

    var recentBooks []models.Work
    currentYear := time.Now().Year()
    cutoffYear := currentYear - 2

    for _, work := range subjectResult.Works {
        // Only include books published in the last two years and exclude future years
        if work.FirstPublishYear >= cutoffYear && work.FirstPublishYear <= currentYear {
            if len(recentBooks) >= 3 {
                break
            }

            workKey := strings.TrimPrefix(work.Key, "/works/")
            description, err := fetchDescription(ctx, workKey)
            if err != nil {
                continue // Skip this book if we can't fetch the description
            }

            var authors []string
            for _, a := range work.Authors {
                authors = append(authors, a.Name)
            }

            // Log the book's title, authors, and publish year
            log.Printf("Chosen Book: %s, Authors: %v, Published Year: %d", work.Title, authors, work.FirstPublishYear)

            recentWork := models.Work{
                Title:       work.Title,
                Authors:     authors,
                Description: description,
            }

            recentBooks = append(recentBooks, recentWork)
        }
    }

    if len(recentBooks) == 0 {
        return nil, fmt.Errorf("no books found for subject '%s' published in the last two years", subject)
    }

    return recentBooks, nil
}


func fetchDescription(ctx context.Context, workKey string) (*string, error) {
    descURL := fmt.Sprintf("https://openlibrary.org/works/%s.json", workKey)
    req, err := http.NewRequestWithContext(ctx, "GET", descURL, nil)
    if err != nil {
        return nil, fmt.Errorf("error creating request for description: %v", err)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("error fetching description: %v", err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("error reading description: %v", err)
    }

    var descResult struct {
        Description interface{} `json:"description"`
    }
    if err := json.Unmarshal(body, &descResult); err != nil {
        return nil, fmt.Errorf("error parsing description JSON: %v", err)
    }

    var description *string
    switch v := descResult.Description.(type) {
    case string:
        description = &v
    case map[string]interface{}:
        if val, ok := v["value"].(string); ok {
            description = &val
        }
    }

    return description, nil
}