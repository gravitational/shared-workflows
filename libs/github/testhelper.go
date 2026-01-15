package github

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"

	go_github "github.com/google/go-github/v71/github"
)

// newFakeClient creates a new GitHub Client using the provided HTTP mux for handling requests.
// This is useful for testing with mock HTTP responses.
func newFakeClient(mux *http.ServeMux) (*Client, func()) {
	srv := httptest.NewServer(mux)
	closer := func() {
		srv.Close()
	}
	cl := go_github.NewClient(srv.Client())
	baseURL, err := url.Parse(srv.URL + "/")
	if err != nil {
		panic("parsing test server URL: " + err.Error())
	}
	cl.BaseURL = baseURL

	return &Client{
		client: cl,
		search: cl.Search,
	}, closer
}

func respondWithJSONTestdata(w http.ResponseWriter, filename string) error {
	f, err := os.Open(filepath.Join("testdata", filename))
	if err != nil {
		return err
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/json")
	_, err = io.Copy(w, f)
	return err
}
