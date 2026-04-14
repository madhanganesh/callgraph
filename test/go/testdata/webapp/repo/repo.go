package repo

import "database/sql"

type Repo struct {
	db *sql.DB
}

func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) SaveOrder(userID int, item string, price int) error {
	_, err := r.db.Exec(
		"INSERT INTO orders(user_id, item, price) VALUES(?, ?, ?)",
		userID, item, price,
	)
	return err
}

func (r *Repo) CleanupExpired() error {
	_, err := r.db.Exec("DELETE FROM orders WHERE expires_at < now()")
	return err
}
