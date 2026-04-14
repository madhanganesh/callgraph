package main

import (
	"database/sql"

	"webapp/controller"
	"webapp/repo"
)

func main() {
	db, _ := sql.Open("sqlite3", "app.db")
	r := repo.New(db)
	c := controller.New(r)

	go c.StartBackgroundJob()
	c.PlaceOrder(42, "widget")
}
