package database

import (
	"database/sql"
	"log"
	"os"
	"fmt"
	"strings"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SetupDatabase initializes the SQLite database and inserts sample data.
func SetupDatabase() {
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

// GetUserFavoriteAuthors retrieves up to five favorite authors for a given user ID.
func GetUserFavoriteAuthors(db *sql.DB, userID int) ([]string, error) {
	row := db.QueryRow("SELECT fauthors FROM users WHERE id = ?", userID)
	var fauthors string
	err := row.Scan(&fauthors)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("User ID %d not found", userID)
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