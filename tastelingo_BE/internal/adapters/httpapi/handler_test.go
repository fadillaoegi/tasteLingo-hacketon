package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"tastelingo/internal/adapters/llm"
	"tastelingo/internal/adapters/memory"
	"tastelingo/internal/adapters/qloo"
	"tastelingo/internal/application"
	"testing"
	"time"
)

func TestOnboardingToPlanOverHTTP(t *testing.T) {
	h := (&Handler{Svc: &application.Service{Repo: memory.New(), Taste: qloo.Mock{}, LLM: llm.Mock{}, Now: time.Now}}).Routes()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	if rec := do("GET", "/api/search?q=s", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("short query should be rejected, got %d", rec.Code)
	}
	rec := do("GET", "/api/search?q=spirited&type=movie", "")
	var search struct {
		Entities []struct{ ID, Name, Type string }
		Source   string
	}
	if json.Unmarshal(rec.Body.Bytes(), &search); rec.Code != 200 || search.Source != "mock" || len(search.Entities) != 1 {
		t.Fatalf("search: %d %s", rec.Code, rec.Body)
	}
	fav := search.Entities[0]
	rec = do("POST", "/api/learners", `{"name":"Rina","level":"B1","goal":"work","daily_minutes":30,"favorites":[{"id":"`+fav.ID+`","name":"`+fav.Name+`","type":"movie"}]}`)
	var learner struct{ ID string }
	if json.Unmarshal(rec.Body.Bytes(), &learner); rec.Code != http.StatusCreated || learner.ID == "" {
		t.Fatalf("create learner: %d %s", rec.Code, rec.Body)
	}
	if rec = do("GET", "/api/learners/"+learner.ID+"/plan", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("no plan yet, got %d", rec.Code)
	}
	rec = do("POST", "/api/learners/"+learner.ID+"/plan", "")
	var built struct {
		Plan struct{ Days []any }
		Run  struct{ ID, Status string }
	}
	if json.Unmarshal(rec.Body.Bytes(), &built); rec.Code != 200 || len(built.Plan.Days) != 7 || built.Run.Status != "succeeded" {
		t.Fatalf("build plan: %d %s", rec.Code, rec.Body)
	}
	if rec = do("GET", "/api/runs/"+built.Run.ID, ""); rec.Code != 200 {
		t.Fatalf("run trace: %d", rec.Code)
	}
}

func TestMountServesUnderBasePath(t *testing.T) {
	h := Mount("/tastelingo/", (&Handler{Svc: &application.Service{Repo: memory.New(), Taste: qloo.Mock{}, LLM: llm.Mock{}, Now: time.Now}}).Routes())
	for path, want := range map[string]int{"/tastelingo/api/health": 200, "/tastelingo": http.StatusFound, "/api/health": 404} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: got %d, want %d", path, rec.Code, want)
		}
	}
}
