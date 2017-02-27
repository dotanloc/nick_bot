package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/jasonlvhit/gocron"
	_ "github.com/mattn/go-sqlite3"

	"github.com/icholy/nick_bot/facebot"
	"github.com/icholy/nick_bot/faceutil"
	"github.com/icholy/nick_bot/imgstore"
	"github.com/icholy/nick_bot/model"
)

var (
	username = flag.String("username", "", "instagram username")
	password = flag.String("password", "", "instagram password")
	minfaces = flag.Int("minfaces", 1, "minimum faces")
	upload   = flag.Bool("upload", false, "enable photo uploading")
	testimg  = flag.String("test.image", "", "test image")
	testdir  = flag.String("test.dir", "", "test a directory of images")
	facedir  = flag.String("face.dir", "faces", "directory to load faces from")
	httpport = flag.String("http.port", "", "http port (example :8080)")

	resetStore = flag.Bool("reset.store", false, "mark all store records as available")
	storefile  = flag.String("store", "store.db", "the store file")

	postNow      = flag.Bool("post.now", false, "post and exit")
	postInterval = flag.Duration("post.interval", 0, "how often to post")
	postTimes    times
)

var banner = `
  _  _ _    _     ___      _
 | \| (_)__| |__ | _ ) ___| |
 | .' | / _| / / | _ \/ _ \  _|
 |_|\_|_\__|_\_\ |___/\___/\__|

 Adding some much needed nick to your photos.
`

func init() {
	flag.Var(&postTimes, "post.time", "time to post")
}

type times []string

func (t *times) String() string {
	return fmt.Sprint(*t)
}

func (t *times) Set(value string) error {
	*t = append(*t, value)
	return nil
}

func main() {
	flag.Parse()

	fmt.Println(banner)

	faceutil.MustLoadFaces(*facedir)

	store, err := imgstore.Open(*storefile)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if *resetStore {
		if err := store.ResetStates(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *testimg != "" {
		if err := testImage(*testimg, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *testdir != "" {
		if err := testImageDir(*testdir); err != nil {
			log.Fatal(err)
		}
		return
	}

	captions, err := readLines("captions.txt")
	if err != nil {
		log.Fatal(err)
	}
	shuffle(captions)

	bot, err := facebot.New(&facebot.Options{
		Username: *username,
		Password: *password,
		MinFaces: *minfaces,
		Upload:   *upload,
		Captions: captions,
		Store:    store,
	})
	if err != nil {
		log.Fatal(err)
	}
	go bot.Run()

	if *httpport != "" {
		startHTTPServer(bot, store)
	}

	doPost := func() {
		if err := bot.Post(); err != nil {
			log.Printf("posting error: %s\n", err)
		}
	}

	switch {
	case *postNow:
		doPost()
		return
	case *postInterval != 0:
		for {
			doPost()
			time.Sleep(*postInterval)
		}
	case len(postTimes) > 0:
		for _, t := range postTimes {
			gocron.Every(1).Day().At(t).Do(doPost)
		}
		<-gocron.Start()
	default:
		select {}
	}
}

func startHTTPServer(bot *facebot.Bot, store *imgstore.Store) {
	http.HandleFunc("/demo", func(w http.ResponseWriter, r *http.Request) {
		img, err := bot.Demo()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "image/jpeg")
		if err := jpeg.Encode(w, img, &jpeg.Options{Quality: jpeg.DefaultQuality}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats, err := store.Stats(model.MediaAvailable)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	go func() {
		if err := http.ListenAndServe(*httpport, nil); err != nil {
			log.Printf("error: %s\n", err)
		}
	}()
}
