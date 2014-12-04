package main

import (
	"bytes"
	"code.google.com/p/goauth2/oauth"
	calendar "code.google.com/p/google-api-go-client/calendar/v3"
	"encoding/gob"
	//"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	var config = &oauth.Config{
		ClientId:     "1019355571230-7e7nbj16j8gif3ilo82ug8ate2k3uurg.apps.googleusercontent.com",
		ClientSecret: "r8bdQQhkF36U6Z1_nzOwXKbg",
		Scope:        calendar.CalendarScope,
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
	}

	client := getOAuthClient(config)

	svc, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to create Calendar service: %v", err)
	}

	c, err := svc.Colors.Get().Do()
	if err != nil {
		log.Fatalf("Unable to retrieve calendar colors: %v", err)
	}

	log.Printf("Kind of colors: %v", c.Kind)
	log.Printf("Colors last updated: %v", c.Updated)

	for k, v := range c.Calendar {
		log.Printf("Calendar[%v]: Background=%v, Foreground=%v", k, v.Background, v.Foreground)
	}

	for k, v := range c.Event {
		log.Printf("Event[%v]: Background=%v, Foreground=%v", k, v.Background, v.Foreground)
	}

	listRes, err := svc.CalendarList.List().Fields("items/id").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve list of calendars: %v", err)
	}
	for _, v := range listRes.Items {
		log.Printf("Calendar ID: %v\n", v.Id)
	}

	mode := "find"

	if len(listRes.Items) > 0 {
		id := listRes.Items[0].Id

		if mode == "find" {
			res, err := svc.Events.List(id).PrivateExtendedProperty("hoge=hogehoge").Fields("items(id,updated,summary,kind)", "summary", "nextPageToken").Do()
			if err != nil {
				log.Fatalf("Unable to retrieve calendar events list: %v", err)
			}
			for _, v := range res.Items {
				log.Printf("Calendar ID %q event: %v(%v) %v: %q\n", id, v.Id, v.Kind, v.Updated, v.Summary)
			}
		}

		if mode == "insert" {
			ep := calendar.EventExtendedProperties{}
			ep.Private = make(map[string]string)
			ep.Private["hoge"] = "hogehoge"
			ep.Shared = make(map[string]string)

			newEvent := calendar.Event{
				Start:              &calendar.EventDateTime{Date: "2014-11-20"},
				End:                &calendar.EventDateTime{Date: "2014-11-21"},
				Summary:            "サマリー",
				Description:        "説明",
				ExtendedProperties: &ep,
			}
			v, err := svc.Events.Insert(id, &newEvent).Do()
			if err != nil {
				log.Fatalf("Unable to insert an event: %v", err)
			}
			log.Printf("Calendar ID %q event: %v(%v) %v: %q\n", id, v.Id, v.Kind, v.Updated, v.Summary)
		}

		if mode == "list" {
			res, err := svc.Events.List(id).OrderBy("updated").Fields("items(id,updated,summary,kind)", "summary", "nextPageToken").Do()
			if err != nil {
				log.Fatalf("Unable to retrieve calendar events list: %v", err)
			}
			for _, v := range res.Items {
				log.Printf("Calendar ID %q event: %v(%v) %v: %q\n", id, v.Id, v.Kind, v.Updated, v.Summary)
			}
			log.Printf("Calendar ID %q Summary: %v\n", id, res.Summary)
			log.Printf("Calendar ID %q next page token: %v\n", id, res.NextPageToken)
		}
	}

}

func getOAuthClient(config *oauth.Config) *http.Client {
	cacheFile := tokenCacheFile(config)
	token, err := tokenFromFile(cacheFile)
	if err != nil {
		token = tokenFromWeb(config)
		saveToken(cacheFile, token)
	} else {
		log.Printf("Using cached token %#v from %q", token, cacheFile)
	}

	t := &oauth.Transport{
		Token:     token,
		Config:    config,
		Transport: condDebugTransport(http.DefaultTransport),
	}
	return t.Client()
}

func osUserCacheDir() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Caches")
	case "linux", "freebsd":
		return filepath.Join(os.Getenv("HOME"), ".cache")
	}
	log.Printf("TODO: osUserCacheDir on GOOS %q", runtime.GOOS)
	return "."
}

func tokenCacheFile(config *oauth.Config) string {
	hash := fnv.New32a()
	hash.Write([]byte(config.ClientId))
	hash.Write([]byte(config.ClientSecret))
	hash.Write([]byte(config.Scope))
	fn := fmt.Sprintf("go-api-demo-tok%v", hash.Sum32())
	return filepath.Join(osUserCacheDir(), url.QueryEscape(fn))
}

func tokenFromFile(file string) (*oauth.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := new(oauth.Token)
	err = gob.NewDecoder(f).Decode(t)
	return t, err
}

func saveToken(file string, token *oauth.Token) {
	f, err := os.Create(file)
	if err != nil {
		log.Printf("Warning: failed to cache oauth token: %v", err)
		return
	}
	defer f.Close()
	gob.NewEncoder(f).Encode(token)
}

func tokenFromWeb(config *oauth.Config) *oauth.Token {
	ch := make(chan string)
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/favicon.ico" {
			http.Error(rw, "", 404)
			return
		}
		if req.URL.Path == "/hello" {
			fmt.Fprintf(rw, "<h1>Hello</h1>")
			rw.(http.Flusher).Flush()
			return
		}
		if req.FormValue("state") != randState {
			log.Printf("State doesn't match: req = %#v", req)
			http.Error(rw, "", 500)
			return
		}
		if code := req.FormValue("code"); code != "" {
			fmt.Fprintf(rw, "<h1>Success</h1>Authorized.")
			rw.(http.Flusher).Flush()
			ch <- code
			return
		}
		log.Printf("no code")
		http.Error(rw, "", 500)
	}))
	defer ts.Close()

	config.RedirectURL = ts.URL
	authUrl := config.AuthCodeURL(randState)
	go openUrl(authUrl)
	log.Printf("Authorize this app at: %s", authUrl)
	code := <-ch
	log.Printf("Got code: %s", code)

	t := &oauth.Transport{
		Config:    config,
		Transport: condDebugTransport(http.DefaultTransport),
	}
	_, err := t.Exchange(code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}
	return t.Token
}

func condDebugTransport(rt http.RoundTripper) http.RoundTripper {
	return &logTransport{rt}
	//return rt
}

func openUrl(url string) {
	err := exec.Command("cmd", "/C", "start", "", strings.Replace(url, "&", "^&", -1)).Run()
	if err == nil {
		return
	}
	//try := []string{"xdg-open", "google-chrome", "open", "cmd /C start"}
	//for _, bin := range try {
	//	err := exec.Command(bin, url).Run()
	//	if err == nil {
	//		return
	//	}
	//}
	log.Printf("Error opening URL in browser.")
}

// debug.go

type logTransport struct {
	rt http.RoundTripper
}

func (t *logTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var buf bytes.Buffer

	os.Stdout.Write([]byte("\n[request]\n"))
	if req.Body != nil {
		req.Body = ioutil.NopCloser(&readButCopy{req.Body, &buf})
	}
	req.Write(os.Stdout)
	if req.Body != nil {
		req.Body = ioutil.NopCloser(&buf)
	}
	os.Stdout.Write([]byte("\n[/request]\n"))

	res, err := t.rt.RoundTrip(req)

	fmt.Printf("[response]\n")
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	} else {
		body := res.Body
		res.Body = nil
		res.Write(os.Stdout)
		if body != nil {
			res.Body = ioutil.NopCloser(&echoAsRead{body})
		}
	}

	return res, err
}

type echoAsRead struct {
	src io.Reader
}

func (r *echoAsRead) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		os.Stdout.Write(p[:n])
	}
	if err == io.EOF {
		fmt.Printf("\n[/response]\n")
	}
	return n, err
}

type readButCopy struct {
	src io.Reader
	dst io.Writer
}

func (r *readButCopy) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.dst.Write(p[:n])
	}
	return n, err
}
