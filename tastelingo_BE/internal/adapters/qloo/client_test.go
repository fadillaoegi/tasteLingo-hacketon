package qloo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
	"testing"
)

// All payloads below are synthetic and shaped after the developer guide; no Qloo responses are stored.
func TestInsightsSendsSignalsAndParsesEntities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" || r.URL.Path != "/v2/insights" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		if q.Get("filter.type") != "urn:entity:podcast" || q.Get("signal.interests.entities") != "a,b" || q.Get("take") != "3" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"success":true,"results":{"entities":[
			{"entity_id":"e1","name":"Synthetic Show","type":"urn:entity:podcast","properties":{"description":"d","release_year":"2019"},"tags":[{"name":"comedy"}],"query":{"affinity":0.8}},
			{"entity_id":"","name":"dropped"}]}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Key: "test-key", HTTP: srv.Client()}
	got, err := c.Insights(context.Background(), application.InsightsQuery{Type: domain.Podcast, EntityIDs: []string{"a", "b"}, Take: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Year != 2019 || got[0].Type != domain.Podcast || got[0].Affinity != 0.8 || got[0].Tags[0] != "comedy" || got[0].Source != domain.SourceQloo {
		t.Fatalf("unexpected entities %+v", got)
	}
}

func TestSearchParsesTypesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"entity_id":"s1","name":"Synthetic Film","types":["urn:entity:movie"],"properties":{"release_year":2001}}]}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Key: "k", HTTP: srv.Client()}
	got, err := c.Search(context.Background(), "film", "", 5)
	if err != nil || len(got) != 1 || got[0].Type != domain.Movie || got[0].Year != 2001 {
		t.Fatalf("unexpected %+v %v", got, err)
	}
}

func TestErrorsAndRedirectsDoNotLeak(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = r.Header.Get("X-Api-Key") != "" }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/tags" {
			http.Redirect(w, r, other.URL, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Key: "secret", HTTP: srv.Client()}
	if _, err := c.Tags(context.Background(), "x", 3); !errors.Is(err, ErrUnavailable) || leaked {
		t.Fatalf("redirect must fail without forwarding key: %v leaked=%v", err, leaked)
	}
	if _, err := c.Search(context.Background(), "x", domain.Movie, 3); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}
