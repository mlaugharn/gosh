package main

import (
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/goji/param"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

var dbp *bolt.DB

type UrlForm struct {
	Url string `param:"url"`
}

// this is necessary because if a url doesn't start with http://
// a 301 redirect will actually just treat the url as a relative url
// and accidentally send the browser to
// http://shortener-domain/s/url instead of http://url/
func add_http(input_url string) string {
	if !strings.HasPrefix(input_url, "http://") {
		return "http://" + input_url
	}
	return input_url
}

// function that handles POSTed urls from form
func new_short_link(w http.ResponseWriter, r *http.Request) {
	// vessel struct for parsed parameters
	var urlform UrlForm

	r.ParseForm()
	err := param.Parse(r.Form, &urlform)

	// this would be the place to check if a URL were bad
	// but this is not a real url shortener
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusBadRequest) // http 400
		return
	}

	// make the url an absolute url
	urlform.Url = add_http(urlform.Url)

	// hash the url using 32-bit fnv-1a
	hasher := fnv.New32a()
	io.WriteString(hasher, urlform.Url)
	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	// store the map [url -> hash]
	(*dbp).Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("urls"))
		if err != nil {
			return err
		}
		return b.Put([]byte(hash), []byte(urlform.Url))
	})
	log.Printf("Added %s -> %s", hash, urlform.Url)
	http.Redirect(w, r, "/display/"+hash, 301)
}

// handles display of url info
// shown after a url is shortened
func display(c web.C, w http.ResponseWriter, r *http.Request) {
	hash := c.URLParams["h"]
	url := []byte("none")
	err := (*dbp).View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("urls"))
		if bucket == nil {
			return fmt.Errorf("Bucket urls not found")
		}
		url = bucket.Get([]byte(hash))
		return nil
	})

	if err != nil {
		log.Fatal(err)
		http.Redirect(w, r, "/404", 404)
		return
	}
	redir_html_template := `<a href="/s/%s">%s</a> -> %s`
	fmt.Fprintf(w, redir_html_template, hash, hash, url)
}

// function that redirects hash urls
func redirect(c web.C, w http.ResponseWriter, r *http.Request) {
	hash := c.URLParams["h"]
	url := []byte("none")
	err := (*dbp).View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("urls"))
		if bucket == nil {
			return fmt.Errorf("Bucket urls not found")
		}
		url = bucket.Get([]byte(hash))
		return nil
	})

	if err != nil {
		log.Fatal(err)
		http.Redirect(w, r, "/404", 404)
		return
	}
	//http.Redirect(w, r, string(url), http.StatusOK)
	http.Redirect(w, r, string(url), 301)
}

// the entry point
func main() {
	// first lets (create, if we need to, then) open a db connection
	db, err := bolt.Open("urls.db", 0600, nil)
	dbp = db
	if err != nil {
		log.Fatal(err)
	}

	// close the db connection when main exits
	defer db.Close()

	goji.Post("/shorten", new_short_link)
	goji.Get("/display/:h", display)
	goji.Get("/s/:h", redirect)

	// make sure to have file serving handler last
	// or else it will match everything
	// use an absolute directory too

	static_files_location := "./static"
	goji.Handle("/*", http.FileServer(http.Dir(static_files_location)))
	goji.Serve()
}
