package facebot

import (
	"fmt"
	"image"
	"log"
	"math/rand"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/icholy/nick_bot/faceutil"
	"github.com/icholy/nick_bot/imgstore"
	"github.com/icholy/nick_bot/instagram"
	"github.com/icholy/nick_bot/model"
)

type Options struct {
	Username   string
	Password   string
	MinFaces   int
	Upload     bool
	AutoFollow bool
	Captions   []string
	Store      *imgstore.Store
}

type Bot struct {
	opt   *Options
	store *imgstore.Store

	captionIndex int
}

func New(o *Options) (*Bot, error) {
	if o.MinFaces < 1 {
		o.MinFaces = 1
	}
	return &Bot{
		opt:   o,
		store: o.Store,
	}, nil
}

func (b *Bot) getCaption(rec *model.Record) string {
	credit := fmt.Sprintf("photocred goes to: @%s", rec.Username)
	captions := b.opt.Captions
	if len(captions) == 0 {
		return credit
	}
	caption := captions[b.captionIndex]
	b.captionIndex++
	if b.captionIndex >= len(captions) {
		b.captionIndex = 0
	}
	return fmt.Sprintf("%s\n\n%s", caption, credit)
}

func (b *Bot) Run() {
	crawler := instagram.NewCrawler(b.opt.Username, b.opt.Password)
	for media := range crawler.Media() {
		if err := b.handleMedia(media); err != nil {
			log.Printf("bot: %s\n", err)
		}
		// sleep up to a minute between image requests
		time.Sleep(time.Second * time.Duration(rand.Intn(60)))
	}
}

func (b *Bot) handleMedia(m *model.Media) error {
	exists, err := b.store.Has(m.ID)
	if err != nil {
		return err
	}
	if exists {
		return b.handleExistingMedia(m)
	} else {
		return b.handleNewMedia(m)
	}
}

func (b *Bot) handleNewMedia(m *model.Media) error {
	// download image
	img, err := fetchImage(m.URL)
	if err != nil {
		return err
	}

	// find the faces
	faces := faceutil.DetectFaces(img)

	// write to store
	return b.store.Put(&model.Record{
		Media:     *m,
		FaceCount: len(faces),
		State:     model.MediaAvailable,
	})
}

func (b *Bot) handleExistingMedia(m *model.Media) error {
	return nil
}

func (b *Bot) Post() error {

	// find the best image
	rec, err := b.store.SearchRandom(b.opt.MinFaces)
	if err != nil {
		return err
	}
	log.Printf("bot: posting %s\n", rec)

	// try to post it
	if err := b.postRecord(rec); err != nil {
		log.Printf("bot: %s\n", err)
		return b.store.SetState(rec.ID, model.MediaRejected)
	} else {
		return b.store.SetState(rec.ID, model.MediaUsed)
	}
}

func (b *Bot) Demo() (image.Image, error) {
	rec, err := b.store.SearchRandom(b.opt.MinFaces)
	if err != nil {
		return nil, err
	}
	img, err := fetchImage(rec.URL)
	if err != nil {
		return nil, err
	}
	newImage := faceutil.ReplaceFaces(img)
	return newImage, nil
}

func (b *Bot) postRecord(rec *model.Record) error {

	// download image
	img, err := fetchImage(rec.URL)
	if err != nil {
		return err
	}

	// replace the faces
	newImage := faceutil.ReplaceFaces(img)
	if err != nil {
		return err
	}

	// save image
	imgpath := filepath.Join("output", rec.ID+".jpeg")
	log.Printf("bot: writing image %s\n", imgpath)
	if err := writeImage(imgpath, newImage); err != nil {
		return err
	}

	if !b.opt.Upload {
		return nil
	}

	// upload photo
	log.Println("bot: uploading photo")
	session, err := instagram.NewSession(b.opt.Username, b.opt.Password)
	if err != nil {
		return err
	}
	defer session.Close()
	caption := b.getCaption(rec)
	if err := session.UploadPhoto(imgpath, caption); err != nil {
		return err
	}

	if b.opt.AutoFollow {
		return b.followRandom(session, rec.ID)
	}
	return nil
}

func (b *Bot) followRandom(s *instagram.Session, userID string) error {
	followers, err := s.GetFollowers(userID)
	if err != nil {
		return err
	}
	if len(followers) == 0 {
		return nil
	}
	user := followers[rand.Intn(len(followers))]
	log.Println("bot: following %s\n", user)
	return s.Follow(user.ID)
}
