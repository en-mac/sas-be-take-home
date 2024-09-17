package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"sync"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

var httpClient = &http.Client{
    Timeout: 10 * time.Second, // Set appropriate timeouts
    Transport: &http.Transport{
        MaxIdleConns:        100,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  false,
    },
}


// Author represents an author with their name, key, and work count.
type Author struct {
	Name      string
	Key       string
	WorkCount int
}

// Work represents a work retrieved from the Open Library API.
type Work struct {
	Title            string   `json:"title"`
	Authors          []Author `json:"authors"`
	Description      string   `json:"description"`
	FirstPublishYear int      `json:"first_publish_year"`
}

func main() {
	// Set up the database
	setupDatabase()

	// Set up the HTTP server
	http.HandleFunc("/recommendations", recommendationsHandler)

	fmt.Println("Server is running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// setupDatabase initializes the SQLite database and inserts sample data.
func setupDatabase() {
	os.Remove("./user.db")
	database, err := sql.Open("sqlite3", "./user.db")
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	// Create users table
	statement, _ := database.Prepare(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY, 
			username TEXT, 
			fauthors TEXT
		)
	`)
	statement.Exec()

	// Insert sample users
	statement, _ = database.Prepare(`
		INSERT INTO users(username, fauthors) VALUES (?, ?)
	`)
	statement.Exec("Sandra", "Andy Weir; Brandon Sanderson; Arthur C. Clarke; Ursula K. Le Guin; H.G. Wells")
	statement.Exec("JDoe", "George R. R. Martin; Robert Jordan; Neil Gaiman; Robin Hobb; Steven Erikson")
	statement.Exec("NonFicFan3", "Patrick Radden Keefe; Jon Krakauer; David Grann; Charles Montgomery; Jeff Speck")
}

// recommendationsHandler handles the /recommendations endpoint.
func recommendationsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	user1IDStr := r.URL.Query().Get("user1")
	user2IDStr := r.URL.Query().Get("user2")

	if user1IDStr == "" || user2IDStr == "" {
		http.Error(w, "Both 'user1' and 'user2' query parameters are required.", http.StatusBadRequest)
		return
	}

	// Validate and convert user IDs
	user1ID, err1 := strconv.Atoi(user1IDStr)
	user2ID, err2 := strconv.Atoi(user2IDStr)

	if err1 != nil || err2 != nil {
		http.Error(w, "User IDs must be valid integers.", http.StatusBadRequest)
		return
	}

	// Open the database
	database, err := sql.Open("sqlite3", "./user.db")
	if err != nil {
		http.Error(w, "Database connection error.", http.StatusInternalServerError)
		return
	}
	defer database.Close()

	// Fetch favorite authors for each user
	user1Authors, err := getUserFavoriteAuthors(database, user1ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(user1Authors) == 0 {
		http.Error(w, fmt.Sprintf("No favorite authors found for user ID %d.", user1ID), http.StatusNotFound)
		return
	}

	user2Authors, err := getUserFavoriteAuthors(database, user2ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(user2Authors) == 0 {
		http.Error(w, fmt.Sprintf("No favorite authors found for user ID %d.", user2ID), http.StatusNotFound)
		return
	}

	// Resolve author names to author keys
	user1AuthorKeys, err := resolveAuthorKeys(user1Authors)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user2AuthorKeys, err := resolveAuthorKeys(user2Authors)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve subjects per author for each user
	user1SubjectAuthorCount, err := getSubjectAuthorCounts(user1AuthorKeys)
	if err != nil {
		http.Error(w, "Error fetching subjects for user 1.", http.StatusInternalServerError)
		return
	}

	user2SubjectAuthorCount, err := getSubjectAuthorCounts(user2AuthorKeys)
	if err != nil {
		http.Error(w, "Error fetching subjects for user 2.", http.StatusInternalServerError)
		return
	}

	// Identify common subjects and select the most prominent one
	commonSubject, err := findMostCommonSubject(user1SubjectAuthorCount, user2SubjectAuthorCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Fetch books in the common subject
	recommendedBooks, err := getRecommendedBooks(commonSubject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare the response
	response := map[string]interface{}{
		"common_subject":  commonSubject,
		"recommendations": recommendedBooks,
	}

	// Send the JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getUserFavoriteAuthors retrieves up to five favorite authors for a given user ID.
func getUserFavoriteAuthors(db *sql.DB, userID int) ([]string, error) {
	row := db.QueryRow("SELECT fauthors FROM users WHERE id = ?", userID)
	var fauthors string
	err := row.Scan(&fauthors)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("User ID %d not found.", userID)
	} else if err != nil {
		return nil, err
	}

	// Split and trim authors
	authors := strings.Split(fauthors, ";")
	var trimmedAuthors []string
	for i, author := range authors {
		if i >= 5 {
			break
		}
		trimmedAuthors = append(trimmedAuthors, strings.TrimSpace(author))
	}
	return trimmedAuthors, nil
}

// resolveAuthorKeys searches for authors and returns their Open Library keys concurrently.
func resolveAuthorKeys(authors []string) ([]Author, error) {
	var (
		authorKeys   []Author
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
			resp, err := httpClient.Get(searchURL)
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
			var selectedAuthor Author
			maxWorkCount := -1
			for _, doc := range result.Docs {
				if doc.WorkCount > maxWorkCount {
					maxWorkCount = doc.WorkCount
					// Ensure the key does not include leading slashes
					key := strings.TrimPrefix(doc.Key, "/authors/")
					selectedAuthor = Author{
						Name:      doc.Name,
						Key:       key,
						WorkCount: doc.WorkCount,
					}
				}
			}

			// Safely append to the authorKeys slice
			mu.Lock()
			authorKeys = append(authorKeys, selectedAuthor)
			mu.Unlock()
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return authorKeys, nil
}

// getSubjectAuthorCounts retrieves subjects per author and counts how many authors have written in each subject concurrently.
func getSubjectAuthorCounts(authors []Author) (map[string]int, error) {
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

		// Capture author to avoid issues with goroutine closure
		author := author

		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release the semaphore slot

			// Fetch works for the author
			worksURL := fmt.Sprintf("https://openlibrary.org/authors/%s/works.json?limit=100", author.Key)

			resp, err := httpClient.Get(worksURL)
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
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return subjectAuthorCount, nil
}

// findMostCommonSubject identifies common subjects between two users and selects the most prominent one.
func findMostCommonSubject(user1Subjects, user2Subjects map[string]int) (string, error) {
	commonSubjects := make(map[string]int)

	// Find common subjects and sum the number of authors
	for subject, count1 := range user1Subjects {
		if count2, exists := user2Subjects[subject]; exists {
			commonSubjects[subject] = count1 + count2
		}
	}

	if len(commonSubjects) == 0 {
		return "", fmt.Errorf("No common subjects found between the users.")
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

// getRecommendedBooks fetches books in the common subject and returns the top three recent books with descriptions fetched concurrently.
func getRecommendedBooks(subject string) ([]Work, error) {
	// Fetch books for the subject
	subjectURL := fmt.Sprintf("https://openlibrary.org/subjects/%s.json?limit=50&sort=new", strings.ReplaceAll(subject, " ", "_"))

	resp, err := httpClient.Get(subjectURL)
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
		recentBooks  []Work
		mu           sync.Mutex
		wg           sync.WaitGroup
		concurrency   = 200 // Limit the number of concurrent goroutines for fetching descriptions
		sem           = make(chan struct{}, concurrency)
		booksToProcess = make([]struct {
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
		}, 0)
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
			var authors []Author
			for _, a := range book.Work.Authors {
				authors = append(authors, Author{
					Name: a.Name,
					Key:  a.Key,
				})
			}

			// Fetch description if available
			description := ""
			workKey := strings.TrimPrefix(book.Work.Key, "/works/")
			descURL := fmt.Sprintf("https://openlibrary.org/works/%s.json", workKey)
			descResp, err := httpClient.Get(descURL)
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
			recentBook := Work{
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