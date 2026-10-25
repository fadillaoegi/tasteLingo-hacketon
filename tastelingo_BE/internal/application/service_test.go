package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"tastelingo/internal/adapters/llm"
	"tastelingo/internal/adapters/memory"
	"tastelingo/internal/adapters/qloo"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
	"testing"
	"time"
)

func newService(model application.LLM) *application.Service {
	return &application.Service{Repo: memory.New(), Taste: qloo.Mock{}, LLM: model, Now: time.Now}
}

func learner(t *testing.T, svc *application.Service) domain.Learner {
	return learnerWith(t, svc, "fav-1")
}

func learnerWith(t *testing.T, svc *application.Service, favoriteID string) domain.Learner {
	t.Helper()
	l, err := svc.CreateLearner(context.Background(), domain.Learner{
		Name: "Rina", Level: "B1", Goal: "work", DailyMinutes: 30,
		Favorites: []domain.Entity{{ID: favoriteID, Name: "Synthetic Favorite", Type: domain.Movie}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestBuildPlanWithMocksProducesGroundedPlanAndTrace(t *testing.T) {
	svc := newService(llm.Mock{})
	l := learner(t, svc)
	plan, run, err := svc.BuildPlan(context.Background(), l.ID)
	if err != nil {
		t.Fatalf("build plan: %v (trace %+v)", err, run.Steps)
	}
	if len(plan.Days) != 7 || plan.Version != 1 || plan.Grounding != domain.GroundingTasteGraph {
		t.Fatalf("unexpected plan %+v", plan)
	}
	for _, d := range plan.Days {
		if d.Entity.Source != domain.SourceMock || !strings.HasSuffix(d.Entity.Name, "(mock)") {
			t.Fatalf("day %d entity is not labelled mock: %+v", d.Day, d.Entity)
		}
	}
	stored, err := svc.Run(context.Background(), run.ID)
	if err != nil || stored.Status != domain.RunSucceeded || stored.TasteMode != "mock" || stored.LLMMode != "mock" {
		t.Fatalf("run not stored as succeeded mock run: %+v %v", stored, err)
	}
	calls := 0
	for _, s := range stored.Steps {
		if s.Kind == domain.StepToolCall {
			calls++
		}
	}
	if calls != 5 {
		t.Fatalf("expected 4 insight calls + build_plan, got %d", calls)
	}
	again, _, err := svc.BuildPlan(context.Background(), l.ID)
	if err != nil || again.Version != 2 {
		t.Fatalf("second plan should be version 2: %+v %v", again, err)
	}
}

// scripted replays fixed model turns so build_plan validation can be observed.
type scripted struct{ turns []application.Message }

func (s *scripted) Mode() string { return "scripted" }
func (s *scripted) Complete(_ context.Context, _ string, history []application.Message, _ []application.ToolSpec) (application.Message, error) {
	i := (len(history) - 1) / 2
	if i >= len(s.turns) {
		return application.Message{Role: application.RoleModel, Text: "giving up"}, nil
	}
	return s.turns[i], nil
}

func call(name string, args any) application.Message {
	raw, _ := json.Marshal(args)
	return application.Message{Role: application.RoleModel, Calls: []application.ToolCall{{ID: name, Name: name, Args: raw}}}
}

func TestBuildPlanRejectsEntitiesNotReturnedByTools(t *testing.T) {
	days := make([]map[string]any, 7)
	for i := range days {
		days[i] = map[string]any{"day": i + 1, "entity_id": "invented-id", "activity": "a", "focus": "reading", "minutes": 10, "why": "w"}
	}
	model := &scripted{turns: []application.Message{call("build_plan", map[string]any{"themes": []string{}, "summary": "s", "days": days})}}
	svc := newService(model)
	l := learner(t, svc)
	_, run, err := svc.BuildPlan(context.Background(), l.ID)
	if !errors.Is(err, application.ErrAgentIncomplete) || run.Status != domain.RunFailed {
		t.Fatalf("expected failed run, got %v %s", err, run.Status)
	}
	found := false
	for _, s := range run.Steps {
		found = found || s.Kind == domain.StepToolResult && strings.Contains(s.Output, "was not returned by any tool")
	}
	if !found {
		t.Fatalf("grounding error not fed back to the model: %+v", run.Steps)
	}
	if _, err := svc.LatestPlan(context.Background(), l.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no plan should be stored, got %v", err)
	}
}

func TestBuildPlanRejectsFavoritesAndOverlongDays(t *testing.T) {
	ctx := context.Background()
	insights, _ := qloo.Mock{}.Insights(ctx, application.InsightsQuery{Type: domain.Movie, Take: 4})
	podcasts, _ := qloo.Mock{}.Insights(ctx, application.InsightsQuery{Type: domain.Podcast, Take: 4})
	ids := []string{insights[0].ID, insights[1].ID, insights[2].ID, insights[3].ID, podcasts[0].ID, podcasts[1].ID, podcasts[2].ID}
	plan := func(minutes int, firstID string) map[string]any {
		days := make([]map[string]any, 7)
		for i := range days {
			days[i] = map[string]any{"day": i + 1, "entity_id": ids[i], "activity": "a", "focus": "reading", "minutes": minutes, "why": "w"}
		}
		days[0]["entity_id"] = firstID
		return map[string]any{"themes": []string{"t"}, "summary": "s", "days": days}
	}
	for _, c := range []struct {
		name, favorite, want string
		args                 map[string]any
	}{
		{"too long", "fav-1", "minutes must be", plan(45, ids[0])},
		{"favorite", ids[6], "own favorite", plan(20, ids[6])},
	} {
		model := &scripted{turns: []application.Message{
			{Role: application.RoleModel, Calls: []application.ToolCall{
				{ID: "1", Name: "get_taste_insights", Args: json.RawMessage(`{"type":"movie","take":4}`)},
				{ID: "2", Name: "get_taste_insights", Args: json.RawMessage(`{"type":"podcast","take":4}`)},
			}},
			call("build_plan", c.args),
		}}
		svc := newService(model)
		_, run, err := svc.BuildPlan(ctx, learnerWith(t, svc, c.favorite).ID)
		if !errors.Is(err, application.ErrAgentIncomplete) || !strings.Contains(lastResult(run), c.want) {
			t.Fatalf("%s: expected %q rejection, got %v", c.name, c.want, err)
		}
	}
	model := &scripted{turns: []application.Message{
		{Role: application.RoleModel, Calls: []application.ToolCall{
			{ID: "1", Name: "get_taste_insights", Args: json.RawMessage(`{"type":"movie","take":4}`)},
			{ID: "2", Name: "get_taste_insights", Args: json.RawMessage(`{"type":"podcast","take":4}`)},
		}},
		call("build_plan", plan(20, ids[0])),
	}}
	svc := newService(model)
	if p, _, err := svc.BuildPlan(ctx, learner(t, svc).ID); err != nil || len(p.Days) != 7 {
		t.Fatalf("valid plan rejected: %v", err)
	}
}

func TestCreateLearnerValidates(t *testing.T) {
	svc := newService(llm.Mock{})
	_, err := svc.CreateLearner(context.Background(), domain.Learner{Name: "R", Level: "Z9", Goal: "work", DailyMinutes: 30})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func lastResult(run domain.AgentRun) string {
	for i := len(run.Steps) - 1; i >= 0; i-- {
		if run.Steps[i].Kind == domain.StepToolResult {
			return run.Steps[i].Output
		}
	}
	return ""
}
