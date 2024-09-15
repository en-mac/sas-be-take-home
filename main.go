package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {

	fmt.Println("Hello and good luck!")

	// Simple Setup
	os.Remove("./user.db")
	database, err := sql.Open("sqlite", "./user.db")
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	statement, _ := database.Prepare("CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT, fauthors TEXT)")
	statement.Exec()

	statement, _ = database.Prepare("INSERT INTO users(username, fauthors) VALUES (?, ?)")
	statement.Exec("Sandra", "Andy Weir; Brandon Sanderson; Arthur Clarke; Ursula Le Guin; H.G. Wells")
	statement.Exec("JDoe", "George R. R. Martin; Robert Jordan; Neil Gaiman; Robin Hobb; Steven Erikson")
	statement.Exec("NonFicFan3", "Patrick Radden Keefe; Jon Krakauer; David Grann; Charles Montgomery; Jeff Speck")

	// Ex: Basic Iterate table
	rows, _ := database.Query("SELECT id, username, fauthors FROM users")
	var id int
	var username string
	var fauthors string

	for rows.Next() {
		rows.Scan(&id, &username, &fauthors)
		fmt.Println(strconv.Itoa(id)+"-"+username+": ", fauthors)
	}

	// Ex: Open Library API Call
	url := "http://openlibrary.org/subjects/fantasy.json?limit=2&sort=new"
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		fmt.Println(err)
		return
	}
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(body))
}
