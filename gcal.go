package main

import (
	"bytes"
	"encoding/gob"
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
	//"errors"

	calendar "code.google.com/p/google-api-go-client/calendar/v3"
	//calendar "github.com/google/google-api-go-client/calendar/v3"

	"code.google.com/p/goauth2/oauth"
	//"golang.org/x/oauth2"
)

// LoginGcal ...
// opens browser and authenticate as a gcal user
func LoginGcal(config *GcalConfig, cacheDirName string) (*calendar.Service, error) {
	var oauthconfig = &oauth.Config{
		ClientId:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Scope:        calendar.CalendarScope,
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
	}

	client := getOAuthClient(oauthconfig, cacheDirName)

	svc, err := calendar.New(client)
	if err != nil {
		return nil, err
	}
	return svc, err
}

// FetchEventByExtendedProperty ...
// to fetch an event corresponding to a Garoon event
func FetchEventByExtendedProperty(gcal *calendar.Service, calendarID string, epexpr string) (*calendar.Event, error) {
	res, err := gcal.Events.List(calendarID).PrivateExtendedProperty(epexpr).Fields("items(id,summary,description,start,end,recurrence,extendedProperties)", "summary", "nextPageToken").Do()
	if err != nil {
		return nil, err
	}
	for _, v := range res.Items {
		//log.Printf("Calendar ID %q event: %v(%v) %v: %q\n", calendarID, v.Id, v.Kind, v.Updated, v.Summary)
		return v, nil
	}

	return nil, nil
}

// FetchGcalEventListByDatetime ...
// fetches events between start and end
func FetchGcalEventListByDatetime(gcal *calendar.Service, calendarID string, start time.Time, end time.Time) (*calendar.Events, error) {
	res, err := gcal.Events.List(calendarID).
		TimeMin(start.Local().Format(time.RFC3339)).
		TimeMax(end.Local().Format(time.RFC3339)).
		Fields("items(id,summary,description,start,end,recurrence,extendedProperties)", "summary", "nextPageToken").
		Do()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func getOAuthClient(oauthconfig *oauth.Config, cacheDirName string) *http.Client {
	cacheFile := tokenCacheFile(cacheDirName, oauthconfig)
	token, err := tokenFromFile(cacheFile)
	if err != nil {
		token = tokenFromWeb(oauthconfig)
		saveToken(cacheFile, token)
	} else {
		//log.Printf("Using cached token %#v from %q", token, cacheFile)
	}

	t := &oauth.Transport{
		Token:     token,
		Config:    oauthconfig,
		Transport: http.DefaultTransport,
		//Transport: condDebugTransport(http.DefaultTransport),
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
	//log.Printf("TODO: osUserCacheDir on GOOS %q", runtime.GOOS)
	return "."
}

func tokenCacheFile(dirName string, oauthconfig *oauth.Config) string {
	hash := fnv.New32a()
	hash.Write([]byte(oauthconfig.ClientId))
	hash.Write([]byte(oauthconfig.ClientSecret))
	hash.Write([]byte(oauthconfig.Scope))
	fn := fmt.Sprintf("go-api-demo-tok%v", hash.Sum32())
	return filepath.Join(dirName, url.QueryEscape(fn))
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

func tokenFromWeb(oauthconfig *oauth.Config) *oauth.Token {
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

	oauthconfig.RedirectURL = ts.URL
	authURL := oauthconfig.AuthCodeURL(randState)
	go openURL(authURL)
	log.Printf("Authorize this app at: %s", authURL)
	code := <-ch
	//log.Printf("Got code: %s", code)

	t := &oauth.Transport{
		Config:    oauthconfig,
		Transport: http.DefaultTransport,
		//Transport: condDebugTransport(http.DefaultTransport),
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

func openURL(url string) {
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
