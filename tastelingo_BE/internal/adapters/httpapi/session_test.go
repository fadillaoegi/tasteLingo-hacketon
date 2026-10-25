package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"tastelingo/internal/adapters/llm"
	"tastelingo/internal/adapters/memory"
	"tastelingo/internal/adapters/qloo"
	"tastelingo/internal/application"
	"testing"
	"time"
)

func TestSessionRestoresMissingUnfinishedAndCompletedProfiles(t *testing.T) {
	svc := &application.Service{Repo: memory.New(), Taste: qloo.Mock{}, LLM: llm.Mock{}, Now: time.Now}
	h := (&Handler{Svc: svc}).Routes()
	get := func(id string) application.LearnerSession {
		t.Helper()
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/api/learners/"+id+"/session", nil))
		if r.Code != 200 {
			t.Fatalf("restore should not produce 404: %d %s", r.Code, r.Body)
		}
		var out application.LearnerSession
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := get("expired-browser-profile"); out.Learner != nil || out.Plan != nil {
		t.Fatalf("stale profile: %+v", out)
	}
	l, err := svc.CreateDemoLearner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out := get(l.ID); out.Learner == nil || out.Plan != nil || out.Run != nil || out.Feedback == nil {
		t.Fatalf("unfinished profile: %+v", out)
	}
	p, run, err := svc.BuildPlan(context.Background(), l.ID)
	if err != nil {
		t.Fatal(err)
	}
	out := get(l.ID)
	if out.Plan == nil || out.Plan.ID != p.ID || out.Run == nil || out.Run.ID != run.ID || len(out.Plan.Days) != 7 {
		t.Fatalf("completed profile: %+v", out)
	}
	if out.Modes["llm"] != "mock" || out.Modes["taste_graph"] != "mock" {
		t.Fatalf("modes lost: %+v", out.Modes)
	}
}
