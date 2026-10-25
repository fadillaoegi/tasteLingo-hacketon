package application

import (
	"context"
	"encoding/json"
	"tastelingo/internal/domain"
)

type Repository interface {
	CreateLearner(context.Context, domain.Learner) error
	GetLearner(context.Context, string) (domain.Learner, error)
	SaveRun(context.Context, domain.AgentRun) error
	GetRun(context.Context, string) (domain.AgentRun, error)
	SavePlan(context.Context, domain.Plan) error
	// LatestPlan returns the newest plan of a learner for one grounding.
	LatestPlan(context.Context, string, string) (domain.Plan, error)
	GetPlan(context.Context, string) (domain.Plan, error)
	SaveExercise(context.Context, domain.Exercise) error
	GetExercise(context.Context, string) (domain.Exercise, error)
	// FindExercise returns the exercise for one plan day, if generated.
	FindExercise(ctx context.Context, planID string, day int) (domain.Exercise, error)
	SaveAttempt(context.Context, domain.Attempt) error
	AddFeedback(context.Context, domain.Feedback) error
	// PlanFeedback lists feedback given on one plan, oldest first.
	PlanFeedback(ctx context.Context, planID string) ([]domain.Feedback, error)
}

type InsightsQuery struct {
	Type      domain.EntityType
	EntityIDs []string
	TagIDs    []string
	Take      int
}

// TasteGraph is the Qloo port. Mode is "qloo" or "mock" and is shown in the UI.
type TasteGraph interface {
	Mode() string
	Search(ctx context.Context, query string, t domain.EntityType, take int) ([]domain.Entity, error)
	Insights(context.Context, InsightsQuery) ([]domain.Entity, error)
	Tags(ctx context.Context, query string, take int) ([]domain.Tag, error)
}

const (
	RoleUser  = "user"
	RoleModel = "model"
	RoleTool  = "tool"
)

type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type ToolResult struct {
	CallID  string          `json:"call_id"`
	Name    string          `json:"name"`
	Content json.RawMessage `json:"content"`
}

// Message is one provider-neutral conversation turn. Raw keeps the provider's
// own model turn (for example Gemini thought signatures) so it can be echoed back.
type Message struct {
	Role    string
	Text    string
	Calls   []ToolCall
	Results []ToolResult
	Raw     json.RawMessage
	// Model identifies the source of Raw when a provider supports model fallback.
	Model string
}

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// LLM is the model port. Mode is the provider name, or "mock".
type LLM interface {
	Mode() string
	Complete(ctx context.Context, system string, history []Message, tools []ToolSpec) (Message, error)
}
