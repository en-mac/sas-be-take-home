package handlers

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "time"

    "be-takehome-2024/internal/database"
    "be-takehome-2024/internal/services"
)

// RecommendationsHandler handles the /recommendations endpoint.
func RecommendationsHandler(w http.ResponseWriter, r *http.Request) {
    // Set a timeout for the request context
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

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

    // Channels to collect subjects and errors
    type subjectResult struct {
        Aggregate map[string]int
        Err       error
    }
    resultsCh := make(chan subjectResult, 2)

    // Fetch subjects for both users concurrently
	go func() {
		// Fetch favorite authors for user1
		user1Authors, err := database.GetUserFavoriteAuthors(db, user1ID)
		if err != nil {
			resultsCh <- subjectResult{nil, fmt.Errorf("User1: %v", err)}
			return
		}
		if len(user1Authors) == 0 {
			resultsCh <- subjectResult{nil, fmt.Errorf("No favorite authors found for user ID %d.", user1ID)}
			return
		}
	
		log.Printf("User1 authors: %v", user1Authors)
	
		// Resolve author keys for user1
		user1AuthorKeys, err := services.ResolveAuthorKeys(ctx, user1Authors)
		if err != nil {
			resultsCh <- subjectResult{nil, fmt.Errorf("User1: %v", err)}
			return
		}
	
		for _, author := range user1AuthorKeys {
			log.Printf("User1 author: Name=%s, Key=%s, WorkCount=%d", author.Name, author.Key, author.WorkCount)
		}
	
		// Get subject counts for user1
		user1SubjectResult, err := services.GetSubjectAuthorCounts(ctx, user1AuthorKeys)
		if err != nil {
			resultsCh <- subjectResult{nil, fmt.Errorf("User1: %v", err)}
			return
		}
		
		resultsCh <- subjectResult{user1SubjectResult.Aggregate, nil}
	}()
	
	go func() {
		// Fetch favorite authors for user2
		user2Authors, err := database.GetUserFavoriteAuthors(db, user2ID)
		if err != nil {
			resultsCh <- subjectResult{nil, fmt.Errorf("User2: %v", err)}
			return
		}
		if len(user2Authors) == 0 {
			resultsCh <- subjectResult{nil, fmt.Errorf("No favorite authors found for user ID %d.", user2ID)}
			return
		}
	
		log.Printf("User2 authors: %v", user2Authors)
	
		// Resolve author keys for user2
		user2AuthorKeys, err := services.ResolveAuthorKeys(ctx, user2Authors)
		if err != nil {
			resultsCh <- subjectResult{nil, fmt.Errorf("User2: %v", err)}
			return
		}
	
		for _, author := range user2AuthorKeys {
			log.Printf("User2 author: Name=%s, Key=%s, WorkCount=%d", author.Name, author.Key, author.WorkCount)
		}
	
		// Get subject counts for user2
		user2SubjectResult, err := services.GetSubjectAuthorCounts(ctx, user2AuthorKeys)
		if err != nil {
			resultsCh <- subjectResult{nil, fmt.Errorf("User2: %v", err)}
			return
		}
		
		resultsCh <- subjectResult{user2SubjectResult.Aggregate, nil}
	}()
	

    // Collect results
    var user1Subjects, user2Subjects map[string]int
    for i := 0; i < 2; i++ {
        select {
        case res := <-resultsCh:
            if res.Err != nil {
                http.Error(w, res.Err.Error(), http.StatusInternalServerError)
                return
            }
            if user1Subjects == nil {
                user1Subjects = res.Aggregate
            } else {
                user2Subjects = res.Aggregate
            }
        case <-ctx.Done():
            http.Error(w, "Request timed out.", http.StatusGatewayTimeout)
            return
        }
    }

    // Find the most common subject
    commonSubject, err := services.FindMostCommonSubject(user1Subjects, user2Subjects)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    log.Printf("Common subject: %s", commonSubject)

    // Fetch recommended books
    recommendedBooks, err := services.GetRecommendedBooks(ctx, commonSubject)
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
