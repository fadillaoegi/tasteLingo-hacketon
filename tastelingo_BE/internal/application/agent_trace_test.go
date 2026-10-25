package application_test

import (
	"context"
	"tastelingo/internal/adapters/llm"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
	"testing"
)

// switching mimics a provider that falls back to another model mid-run.
type switching struct{}

func (switching) Mode() string { return "switching" }
func (switching) Complete(ctx context.Context, system string, history []application.Message, tools []application.ToolSpec) (application.Message, error) {
	msg, err := llm.Mock{}.Complete(ctx, system, history, tools)
	msg.Model = "model-a"
	if len(history) > 1 {
		msg.Model = "model-b"
	}
	return msg, err
}

func TestTraceRecordsAnsweringModel(t *testing.T) {
	svc := newService(switching{})
	_, run, err := svc.BuildPlan(context.Background(), learner(t, svc).ID)
	if err != nil {
		t.Fatal(err)
	}
	models := map[string]string{}
	for _, s := range run.Steps {
		if s.Kind == domain.StepToolCall {
			models[s.Tool] = s.Model
		}
	}
	if models["get_taste_insights"] != "model-a" || models["build_plan"] != "model-b" {
		t.Fatalf("trace should show the model behind each call, got %v", models)
	}
}
