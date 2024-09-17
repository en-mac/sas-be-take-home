// handlers/recommendations.go

package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"be-takehome-2024/internal/database"
	"be-takehome-2024/internal/services"
)

// RecommendationsHandler handles the /recommendations endpoint.
func RecommendationsHandler(w http.ResponseWriter, r *http.Request) {
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
	db, err := sql.Open("sqlite3", "./user.db")
	if err != nil {
		http.Error(w, "Database connection error.", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Fetch favorite authors for each user
	user1Authors, err := database.GetUserFavoriteAuthors(db, user1ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(user1Authors) == 0 {
		http.Error(w, fmt.Sprintf("No favorite authors found for user ID %d.", user1ID), http.StatusNotFound)
		return
	}

	user2Authors, err := database.GetUserFavoriteAuthors(db, user2ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(user2Authors) == 0 {
		http.Error(w, fmt.Sprintf("No favorite authors found for user ID %d.", user2ID), http.StatusNotFound)
		return
	}

	// Resolve author names to author keys
	user1AuthorKeys, err := services.ResolveAuthorKeys(user1Authors)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user2AuthorKeys, err := services.ResolveAuthorKeys(user2Authors)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve subjects per author for each user
	user1SubjectResult, err := services.GetSubjectAuthorCounts(user1AuthorKeys)
	if err != nil {
		http.Error(w, "Error fetching subjects for user 1.", http.StatusInternalServerError)
		return
	}

	user2SubjectResult, err := services.GetSubjectAuthorCounts(user2AuthorKeys)
	if err != nil {
		http.Error(w, "Error fetching subjects for user 2.", http.StatusInternalServerError)
		return
	}

	// Log per-author subjects for User 1
	for author, subjects := range user1SubjectResult.PerAuthor {
		log.Printf("User 1, Author: %s, subjects: %v", author, subjects)
	}

	// Log per-author subjects for User 2
	for author, subjects := range user2SubjectResult.PerAuthor {
		log.Printf("User 2, Author: %s, subjects: %v", author, subjects)
	}

	// Optional: Log aggregate subjects if needed
	// log.Printf("User 1 aggregate subjects: %v", user1SubjectResult.Aggregate)
	// log.Printf("User 2 aggregate subjects: %v", user2SubjectResult.Aggregate)

	// Identify common subjects and select the most prominent one
	commonSubject, err := services.FindMostCommonSubject(user1SubjectResult.Aggregate, user2SubjectResult.Aggregate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Printf("Common subject: %s", commonSubject)

	// Fetch books in the common subject
	recommendedBooks, err := services.GetRecommendedBooks(commonSubject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare the response
	response := map[string]interface{}{
		// "common_subject":  commonSubject,
		"recommendations": recommendedBooks,
	}

	// Send the JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
