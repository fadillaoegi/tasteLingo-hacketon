package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"tastelingo/internal/domain"
)

// The baseline gets the same learner facts as the agent but no taste-graph tools,
// so the comparison panel isolates what Qloo adds.
const llmOnlyPrompt = `You are an English tutor. Recommend English-language films, TV shows, books,
podcasts or music for this learner and turn them into a 7-day plan.
Reply with JSON only, no prose, in this shape:
{"themes": [string], "summary": string,
 "days": [{"day": 1-7, "title": string, "type": "movie|tv_show|book|podcast|artist",
           "activity": string, "focus": "vocabulary|listening|reading|speaking|writing",
           "minutes": int, "why": string}]}`

func (s *Service) LatestComparison(ctx context.Context, learnerID string) (domain.Plan, error) {
	return s.Repo.LatestPlan(ctx, learnerID, domain.GroundingLLMOnly)
}

// BuildComparison asks the LLM alone for a plan. Its titles are stored as
// unverified (source "llm") and never mixed with taste-graph entities.
func (s *Service) BuildComparison(ctx context.Context, learnerID string) (domain.Plan, domain.AgentRun, error) {
	learner, err := s.Repo.GetLearner(ctx, learnerID)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	version, err := s.nextVersion(ctx, learnerID, domain.GroundingLLMOnly)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	run, err := s.startRun(ctx, learnerID, "llm_only_plan")
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	favorites := make([]map[string]any, 0, len(learner.Favorites))
	for _, f := range learner.Favorites {
		favorites = append(favorites, map[string]any{"name": f.Name, "type": f.Type})
	}
	task, _ := json.Marshal(map[string]any{
		"task":    "llm_only",
		"learner": map[string]any{"name": learner.Name, "level": learner.Level, "goal": learner.Goal, "daily_minutes": learner.DailyMinutes, "favorites": favorites},
	})
	plan := domain.Plan{ID: NewID(), LearnerID: learnerID, RunID: run.ID, Version: version, Grounding: domain.GroundingLLMOnly}
	text, runErr := s.agent().Ask(ctx, &run, llmOnlyPrompt, string(task))
	if runErr == nil {
		runErr = parseLLMOnlyPlan(text, &plan)
	}
	saveCtx, cancel, err := s.finishRun(ctx, &run, runErr)
	defer cancel()
	if err != nil {
		return domain.Plan{}, run, err
	}
	if runErr != nil {
		return domain.Plan{}, run, runErr
	}
	plan.CreatedAt = *run.FinishedAt
	return plan, run, s.Repo.SavePlan(saveCtx, plan)
}

func parseLLMOnlyPlan(text string, plan *domain.Plan) error {
	var in struct {
		Themes  []string `json:"themes"`
		Summary string   `json:"summary"`
		Days    []struct {
			Day      int               `json:"day"`
			Title    string            `json:"title"`
			Type     domain.EntityType `json:"type"`
			Activity string            `json:"activity"`
			Focus    string            `json:"focus"`
			Minutes  int               `json:"minutes"`
			Why      string            `json:"why"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(extractJSON(text)), &in); err != nil {
		return errors.New("LLM-only reply was not valid plan JSON")
	}
	for i, d := range in.Days {
		if i == 7 {
			break
		}
		if strings.TrimSpace(d.Title) == "" {
			continue
		}
		if !domain.ValidEntityType(d.Type) {
			d.Type = domain.Movie
		}
		plan.Days = append(plan.Days, domain.PlanDay{
			Day: len(plan.Days) + 1, Activity: d.Activity, Focus: d.Focus, Minutes: d.Minutes, Why: d.Why,
			Entity: domain.Entity{ID: fmt.Sprintf("llm-%d", len(plan.Days)+1), Name: d.Title, Type: d.Type, Source: domain.SourceLLM},
		})
	}
	if len(plan.Days) == 0 {
		return errors.New("LLM-only reply contained no days")
	}
	plan.Themes, plan.Summary = in.Themes, in.Summary
	return nil
}

// extractJSON tolerates code fences or prose around the first JSON object.
func extractJSON(text string) string {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return text
	}
	return text[start : end+1]
}
