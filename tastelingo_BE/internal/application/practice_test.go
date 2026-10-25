package application_test

import (
	"context"
	"errors"
	"strings"
	"tastelingo/internal/adapters/llm"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
	"testing"
)

func TestComparisonIsStoredSeparatelyAsUnverified(t *testing.T) {
	ctx := context.Background()
	svc := newService(llm.Mock{})
	l := learner(t, svc)
	cmp, run, err := svc.BuildComparison(ctx, l.ID)
	if err != nil {
		t.Fatalf("comparison: %v %+v", err, run.Steps)
	}
	if cmp.Grounding != domain.GroundingLLMOnly || len(cmp.Days) != 7 || run.Task != "llm_only_plan" {
		t.Fatalf("unexpected comparison %+v", cmp)
	}
	for _, d := range cmp.Days {
		if d.Entity.Source != domain.SourceLLM {
			t.Fatalf("LLM-only title must be marked unverified: %+v", d.Entity)
		}
	}
	if _, err := svc.LatestPlan(ctx, l.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("comparison must not become the learner's plan: %v", err)
	}
	if got, err := svc.LatestComparison(ctx, l.ID); err != nil || got.ID != cmp.ID {
		t.Fatalf("latest comparison: %v", err)
	}
}

func TestComparisonRejectsNonJSONReply(t *testing.T) {
	svc := newService(&scripted{turns: []application.Message{{Role: application.RoleModel, Text: "Sure! Watch Friends."}}})
	_, run, err := svc.BuildComparison(context.Background(), learner(t, svc).ID)
	if err == nil || run.Status != domain.RunFailed {
		t.Fatalf("expected failed run, got %v %s", err, run.Status)
	}
}

func TestExerciseIsGeneratedOnceAndScored(t *testing.T) {
	ctx := context.Background()
	svc := newService(llm.Mock{})
	plan, _, err := svc.BuildPlan(ctx, learner(t, svc).ID)
	if err != nil {
		t.Fatal(err)
	}
	ex, run, err := svc.Exercise(ctx, plan.ID, 2)
	if err != nil || run == nil || len(ex.Vocabulary) < 5 || len(ex.Questions) != 3 || ex.EntityID != plan.Days[1].Entity.ID {
		t.Fatalf("exercise: %+v %v", ex, err)
	}
	if hidden := ex.ForLearner(); hidden.Questions[0].Answer != nil || ex.Questions[0].Answer == nil {
		t.Fatal("ForLearner must hide answers without changing the original")
	}
	for _, s := range run.Steps {
		if strings.Contains(string(s.Input), `"answer"`) {
			t.Fatalf("answer key leaked into the trace: %s", s.Input)
		}
	}
	again, run2, err := svc.Exercise(ctx, plan.ID, 2)
	if err != nil || again.ID != ex.ID || run2 != nil {
		t.Fatalf("second request should reuse the stored exercise: %v", err)
	}
	if _, _, err := svc.Exercise(ctx, plan.ID, 9); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("day out of range: %v", err)
	}
	if _, _, err := svc.Attempt(ctx, ex.ID, []int{0}, ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("missing answers must be rejected: %v", err)
	}
	at, _, err := svc.Attempt(ctx, ex.ID, []int{0, 0, 0}, "I learned to be brave.")
	if err != nil || at.Score != 2 || !at.Correct[0] || at.Correct[1] || !strings.HasPrefix(at.Feedback, "[mock]") {
		t.Fatalf("attempt: %+v %v", at, err)
	}
}

func TestExerciseRejectsInvalidAnswerKey(t *testing.T) {
	ctx := context.Background()
	bad := map[string]any{
		"vocabulary": []map[string]string{{"term": "a", "meaning": "b", "example": "c"}, {"term": "a", "meaning": "b", "example": "c"}, {"term": "a", "meaning": "b", "example": "c"}, {"term": "a", "meaning": "b", "example": "c"}, {"term": "a", "meaning": "b", "example": "c"}},
		"questions":  []map[string]any{{"question": "q", "options": []string{"x", "y", "z"}, "answer": 5}, {"question": "q", "options": []string{"x", "y", "z"}, "answer": 0}},
		"prompt":     map[string]string{"kind": "writing", "text": "t"},
	}
	planner := newService(llm.Mock{})
	plan, _, err := planner.BuildPlan(ctx, learner(t, planner).ID)
	if err != nil {
		t.Fatal(err)
	}
	svc := &application.Service{Repo: planner.Repo, Taste: planner.Taste, LLM: &scripted{turns: []application.Message{call("generate_exercise", bad)}}, Now: planner.Now}
	_, run, err := svc.Exercise(ctx, plan.ID, 1)
	if !errors.Is(err, application.ErrAgentIncomplete) || !strings.Contains(lastResult(*run), "valid answer index") {
		t.Fatalf("expected answer-key rejection, got %v", err)
	}
}

func TestFeedbackAdaptsPlanAndDropsDislikes(t *testing.T) {
	ctx := context.Background()
	svc := newService(llm.Mock{})
	l := learner(t, svc)
	plan, _, err := svc.BuildPlan(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.AdaptPlan(ctx, l.ID); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("adapting without feedback must be rejected: %v", err)
	}
	if _, err := svc.GiveFeedback(ctx, "someone-else", plan.ID, 1, "dislike"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("feedback on another learner's plan: %v", err)
	}
	if _, err := svc.GiveFeedback(ctx, l.ID, plan.ID, 1, "meh"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid kind: %v", err)
	}
	disliked := plan.Days[0].Entity.ID
	if _, err := svc.GiveFeedback(ctx, l.ID, plan.ID, 1, "dislike"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GiveFeedback(ctx, l.ID, plan.ID, 2, "too_hard"); err != nil {
		t.Fatal(err)
	}
	adapted, run, err := svc.AdaptPlan(ctx, l.ID)
	if err != nil {
		t.Fatalf("adapt: %v %+v", err, run.Steps)
	}
	if adapted.Version != 2 || adapted.Changes == "" || run.Task != "adapt_plan" {
		t.Fatalf("unexpected adapted plan %+v", adapted)
	}
	for _, d := range adapted.Days {
		if d.Entity.ID == disliked {
			t.Fatal("disliked entity was scheduled again")
		}
	}
}

func TestAdaptRejectsDislikedEntity(t *testing.T) {
	ctx := context.Background()
	base := newService(llm.Mock{})
	l := learner(t, base)
	plan, _, err := base.BuildPlan(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.GiveFeedback(ctx, l.ID, plan.ID, 1, "dislike"); err != nil {
		t.Fatal(err)
	}
	days := make([]map[string]any, 7)
	for i, d := range plan.Days {
		days[i] = map[string]any{"day": d.Day, "entity_id": d.Entity.ID, "activity": "a", "focus": "reading", "minutes": 20, "why": "w"}
	}
	model := &scripted{turns: []application.Message{call("adapt_plan", map[string]any{"themes": []string{}, "summary": "s", "changes": "c", "days": days})}}
	svc := &application.Service{Repo: base.Repo, Taste: base.Taste, LLM: model, Now: base.Now}
	_, run, err := svc.AdaptPlan(ctx, l.ID)
	if !errors.Is(err, application.ErrAgentIncomplete) || !strings.Contains(lastResult(run), "disliked") {
		t.Fatalf("expected dislike rejection, got %v", err)
	}
}

func TestDemoLearnerResolvesFavoritesThroughTasteGraph(t *testing.T) {
	svc := newService(llm.Mock{})
	l, err := svc.CreateDemoLearner(context.Background())
	if err != nil || l.Name != "Rina" || len(l.Favorites) != 3 {
		t.Fatalf("demo learner: %+v %v", l, err)
	}
}
