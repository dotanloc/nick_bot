package instagram

import (
	"fmt"
	"log"
	"time"

	"github.com/icholy/nick_bot/model"
)

type Crawler struct {
	username string
	password string
	interval time.Duration

	users     []*model.User
	userIndex int

	out  chan *model.Media
	stop chan struct{}
}

func NewCrawler(username, password string) *Crawler {
	c := &Crawler{
		username: username,
		password: password,
		interval: 10 * time.Minute,

		out:  make(chan *model.Media),
		stop: make(chan struct{}),
	}
	go c.loop()
	return c
}

func (c *Crawler) Media() <-chan *model.Media {
	return c.out
}

func (c *Crawler) loop() {
	for {
		if err := c.crawl(); err != nil {
			log.Printf("crawler: %s\n", err)
		}
		time.Sleep(c.interval)
	}
}

func (c *Crawler) crawl() error {
	s, err := NewSession(c.username, c.password)
	if err != nil {
		return err
	}
	defer s.Close()
	user, err := c.getNextUser(s)
	if err != nil {
		return err
	}
	medias, err := s.GetRecentUserMedias(user)
	if err != nil {
		return err
	}
	for _, media := range medias {
		c.out <- media
	}
	return nil
}

func (c *Crawler) getNextUser(s *Session) (*model.User, error) {

	// fetch the users again if we've used them all or there are none
	if c.userIndex >= len(c.users) {
		users, err := s.GetUsers()
		if err != nil {
			return nil, err
		}
		c.users = users
	}

	if len(c.users) == 0 {
		return nil, fmt.Errorf("no users")
	}

	user := c.users[c.userIndex]
	c.userIndex++
	return user, nil
}
