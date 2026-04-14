package controller

import (
	"net/http"

	"webapp/repo"
)

type Controller struct {
	repo *repo.Repo
}

func New(r *repo.Repo) *Controller {
	return &Controller{repo: r}
}

func (c *Controller) PlaceOrder(userID int, item string) error {
	price, err := c.fetchPrice(item)
	if err != nil {
		return err
	}
	return c.repo.SaveOrder(userID, item, price)
}

func (c *Controller) fetchPrice(item string) (int, error) {
	_, err := http.Get("https://api.example.com/price?item=" + item)
	if err != nil {
		return 0, err
	}
	return 100, nil
}

func (c *Controller) StartBackgroundJob() {
	c.repo.CleanupExpired()
}
