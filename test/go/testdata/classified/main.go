package main

import (
	"database/sql"
	"net/http"
)

func handle(db *sql.DB) {
	http.Get("https://example.com")
	db.Exec("UPDATE t SET x=1")
	go worker()
}

func worker() {
	println("background")
}

func main() {
	handle(nil)
}
