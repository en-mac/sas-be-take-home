package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Author represents an author with their name, key, and work count.
type Author struct {
	Name      string
	Key       string
	WorkCount int
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
	statement.Exec("Sandra", "Andy Weir; Brandon Sanderson; Arthur Clarke; Ursula Le Guin; H.G. Wells")
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

	// Prepare the response
	response := map[string]interface{}{
		"user1_authors": user1AuthorKeys,
		"user2_authors": user2AuthorKeys,
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

// resolveAuthorKeys searches for authors and returns their Open Library keys.
func resolveAuthorKeys(authors []string) ([]Author, error) {
	var authorKeys []Author

	for _, authorName := range authors {
		// Replace spaces with '+' for URL encoding
		queryName := strings.ReplaceAll(authorName, " ", "+")

		// Open Library Search API URL
		searchURL := fmt.Sprintf("https://openlibrary.org/search/authors.json?q=%s", queryName)

		// Make the API request
		resp, err := http.Get(searchURL)
		if err != nil {
			log.Printf("Error fetching data for author '%s': %v", authorName, err)
			continue
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response for author '%s': %v", authorName, err)
			continue
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
			continue
		}

		// No authors found
		if len(result.Docs) == 0 {
			log.Printf("No authors found for '%s'.", authorName)
			continue
		}

		// Select the author with the highest work_count
		var selectedAuthor Author
		maxWorkCount := -1
		for _, doc := range result.Docs {
			if doc.WorkCount > maxWorkCount {
				maxWorkCount = doc.WorkCount
				selectedAuthor = Author{
					Name:      doc.Name,
					Key:       doc.Key,
					WorkCount: doc.WorkCount,
				}
			}
		}

		authorKeys = append(authorKeys, selectedAuthor)
	}

	return authorKeys, nil
}
