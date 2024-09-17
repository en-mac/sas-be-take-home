package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"be-takehome-2024/internal/database"
	"be-takehome-2024/internal/handlers"
)

func main() {
	startTime := time.Now()

	// Set up the database
	database.SetupDatabase()

	// Set up the HTTP server
	http.HandleFunc("/recommendations", func(w http.ResponseWriter, r *http.Request) {
		requestStart := time.Now()
		handlers.RecommendationsHandler(w, r)
		requestDuration := time.Since(requestStart)
		log.Printf("Request processed in %v", requestDuration)
	})

	fmt.Println("Server is running on port 8080...")

	go func() {
		time.Sleep(100 * time.Millisecond) // Give the server a moment to start
		totalSetupTime := time.Since(startTime)
		fmt.Printf("Total setup time: %v\n", totalSetupTime)
	}()

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		totalRunTime := time.Since(startTime)
		log.Printf("Server stopped after running for %v. Error: %v", totalRunTime, err)
	}
}