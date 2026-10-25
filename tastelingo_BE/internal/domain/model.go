package domain

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrValidation = errors.New("validation failed")
)

// EntityType is the short form of a Qloo entity URN (urn:entity:<type>).
type EntityType string

const (
	Movie   EntityType = "movie"
	TVShow  EntityType = "tv_show"
	Book    EntityType = "book"
	Podcast EntityType = "podcast"
	Artist  EntityType = "artist"
)

var MediaTypes = []EntityType{Movie, TVShow, Book, Podcast, Artist}

func ValidEntityType(t EntityType) bool {
	for _, m := range MediaTypes {
		if m == t {
			return true
		}
	}
	return false
}

// Source records where an entity came from so mock data is never shown as Qloo data.
const (
	SourceQloo = "qloo"
	SourceMock = "mock"
	SourceUser = "user"
	// SourceLLM marks a title suggested by the LLM alone, never checked against the taste graph.
	SourceLLM = "llm"
)

type Entity struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        EntityType `json:"type"`
	Description string     `json:"description,omitempty"`
	Year        int        `json:"year,omitempty"`
	ImageURL    string     `json:"image_url,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Affinity    float64    `json:"affinity,omitempty"`
	Source      string     `json:"source"`
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

var (
	Levels       = []string{"A2", "B1", "B2", "C1"}
	Goals        = []string{"conversation", "work", "academic"}
	DailyMinutes = []int{15, 30, 60}
)

type Learner struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Level        string    `json:"level"`
	Goal         string    `json:"goal"`
	DailyMinutes int       `json:"daily_minutes"`
	Favorites    []Entity  `json:"favorites"`
	CreatedAt    time.Time `json:"created_at"`
}

func (l Learner) Validate() error {
	if l.Name == "" || len(l.Name) > 60 {
		return errors.Join(ErrValidation, errors.New("name must be 1-60 characters"))
	}
	if !contains(Levels, l.Level) {
		return errors.Join(ErrValidation, errors.New("level must be one of A2, B1, B2, C1"))
	}
	if !contains(Goals, l.Goal) {
		return errors.Join(ErrValidation, errors.New("goal must be conversation, work or academic"))
	}
	if !contains(DailyMinutes, l.DailyMinutes) {
		return errors.Join(ErrValidation, errors.New("daily_minutes must be 15, 30 or 60"))
	}
	if len(l.Favorites) < 1 || len(l.Favorites) > 5 {
		return errors.Join(ErrValidation, errors.New("pick 1-5 favorites"))
	}
	for _, f := range l.Favorites {
		if f.ID == "" || f.Name == "" || !ValidEntityType(f.Type) {
			return errors.Join(ErrValidation, errors.New("each favorite needs id, name and a supported type"))
		}
	}
	return nil
}

func contains[T comparable](list []T, v T) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

type PlanDay struct {
	Day      int    `json:"day"`
	Entity   Entity `json:"entity"`
	Activity string `json:"activity"`
	Focus    string `json:"focus"`
	Minutes  int    `json:"minutes"`
	Why      string `json:"why"`
}

// Grounding tells whether plan items were verified against taste-graph results.
const (
	GroundingTasteGraph = "taste_graph"
	GroundingLLMOnly    = "llm_only"
)

type Plan struct {
	ID        string    `json:"id"`
	LearnerID string    `json:"learner_id"`
	RunID     string    `json:"run_id"`
	Version   int       `json:"version"`
	Grounding string    `json:"grounding"`
	Themes    []string  `json:"themes"`
	Summary   string    `json:"summary"`
	Days      []PlanDay `json:"days"`
	Changes   string    `json:"changes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Trace step kinds shown in the agent trace panel.
const (
	StepThought    = "thought"
	StepToolCall   = "tool_call"
	StepToolResult = "tool_result"
	StepFinal      = "final"
	StepError      = "error"
)

type TraceStep struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	Tool  string `json:"tool,omitempty"`
	// Model is the LLM that produced this step, so model fallback is visible in the trace.
	Model      string          `json:"model,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     string          `json:"output,omitempty"`
	DurationMS int64           `json:"duration_ms,omitempty"`
	At         time.Time       `json:"at"`
}

const (
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
)

type AgentRun struct {
	ID         string      `json:"id"`
	LearnerID  string      `json:"learner_id"`
	Task       string      `json:"task"`
	Status     string      `json:"status"`
	TasteMode  string      `json:"taste_mode"`
	LLMMode    string      `json:"llm_mode"`
	Steps      []TraceStep `json:"steps"`
	Error      string      `json:"error,omitempty"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
}
