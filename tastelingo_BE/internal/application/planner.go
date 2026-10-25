package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"tastelingo/internal/domain"
)

func (s *Service) LatestPlan(ctx context.Context, learnerID string) (domain.Plan, error) {
	return s.Repo.LatestPlan(ctx, learnerID, domain.GroundingTasteGraph)
}

// BuildPlan runs the planning agent and stores both the plan and its trace.
// A failed run is stored too, so the trace explains what went wrong.
func (s *Service) BuildPlan(ctx context.Context, learnerID string) (domain.Plan, domain.AgentRun, error) {
	learner, err := s.Repo.GetLearner(ctx, learnerID)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	return s.plan(ctx, learner, nil)
}

// AdaptPlan replans from the latest plan and the feedback given on it.
func (s *Service) AdaptPlan(ctx context.Context, learnerID string) (domain.Plan, domain.AgentRun, error) {
	learner, err := s.Repo.GetLearner(ctx, learnerID)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	prev, err := s.LatestPlan(ctx, learnerID)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	feedback, err := s.Repo.PlanFeedback(ctx, prev.ID)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	if len(feedback) == 0 {
		return domain.Plan{}, domain.AgentRun{}, errors.Join(domain.ErrValidation, errors.New("give feedback on at least one day first"))
	}
	return s.plan(ctx, learner, &adaptation{previous: prev, feedback: feedback})
}

type adaptation struct {
	previous domain.Plan
	feedback []domain.Feedback
}

func (a *adaptation) disliked() map[string]bool {
	out := map[string]bool{}
	for _, f := range a.feedback {
		if f.Kind == "dislike" {
			out[f.EntityID] = true
		}
	}
	return out
}

func (s *Service) plan(ctx context.Context, learner domain.Learner, adapt *adaptation) (domain.Plan, domain.AgentRun, error) {
	version, err := s.nextVersion(ctx, learner.ID, domain.GroundingTasteGraph)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	task := "build_plan"
	if adapt != nil {
		task = "adapt_plan"
	}
	run, err := s.startRun(ctx, learner.ID, task)
	if err != nil {
		return domain.Plan{}, domain.AgentRun{}, err
	}
	p := &planner{svc: s, learner: learner, adapt: adapt, seen: map[string]domain.Entity{}}
	p.plan = domain.Plan{ID: NewID(), LearnerID: learner.ID, RunID: run.ID, Version: version, Grounding: domain.GroundingTasteGraph}
	runErr := s.agent().Run(ctx, &run, plannerPrompt, p.task(), p.tools())
	if runErr == nil && !p.built {
		runErr = ErrAgentIncomplete
	}
	saveCtx, cancel, err := s.finishRun(ctx, &run, runErr)
	defer cancel()
	if err != nil {
		return domain.Plan{}, run, err
	}
	if runErr != nil {
		return domain.Plan{}, run, runErr
	}
	p.plan.CreatedAt = *run.FinishedAt
	return p.plan, run, s.Repo.SavePlan(saveCtx, p.plan)
}

const plannerPrompt = `You are TasteLingo, an English-learning planner for non-native learners.
Goal: build a 7-day plan of English-language films, TV shows, books, podcasts and music
that match the learner's real taste, at a difficulty that fits their CEFR level.

Rules:
- Use get_taste_insights (and get_tags or search_entities if useful) to discover content.
  The learner's favorites are signals; do not schedule the favorites themselves.
- Only schedule entities returned by those tools in this conversation, using their exact id.
  Never invent titles. Prefer English-language works or works with a well-known English version.
- Mix at least two content types. Each day's minutes must not exceed the learner's daily_minutes.
- "activity" is a concrete task (for example: watch one episode with English subtitles,
  note 10 phrases). "focus" is one of: vocabulary, listening, reading, speaking, writing.
- "why" explains in one sentence how the item connects to the learner's taste, citing the
  favorite or tag it relates to. Keep the learner's goal in mind.
- When the plan is ready, call build_plan exactly once with themes, summary and 7 days.

When the request is to adapt an existing plan:
- The previous plan's entities may be kept by id, except those the learner disliked.
- "dislike": replace that item and move away from its genre; "like": find more like it
  (its id is already a signal); "too_hard": choose easier formats or lighter activities
  (subtitles, shorter segments, slower speech); "too_easy": raise the challenge.
- Call adapt_plan exactly once, and explain in "changes" what changed and why.`

type planner struct {
	svc     *Service
	learner domain.Learner
	adapt   *adaptation
	seen    map[string]domain.Entity
	plan    domain.Plan
	built   bool
}

func (p *planner) task() string {
	in := map[string]any{
		"task":    "plan",
		"request": "Build a 7-day English learning plan for this learner.",
		"learner": map[string]any{"name": p.learner.Name, "level": p.learner.Level, "goal": p.learner.Goal, "daily_minutes": p.learner.DailyMinutes, "favorites": p.learner.Favorites},
	}
	if p.adapt != nil {
		disliked := p.adapt.disliked()
		days := make([]map[string]any, 0, 7)
		for _, d := range p.adapt.previous.Days {
			if !disliked[d.Entity.ID] {
				p.seen[d.Entity.ID] = d.Entity
			}
			days = append(days, map[string]any{"day": d.Day, "entity_id": d.Entity.ID, "name": d.Entity.Name, "type": d.Entity.Type, "focus": d.Focus, "minutes": d.Minutes})
		}
		feedback := make([]map[string]any, 0, len(p.adapt.feedback))
		for _, f := range p.adapt.feedback {
			feedback = append(feedback, map[string]any{"day": f.Day, "entity_id": f.EntityID, "name": f.EntityName, "kind": f.Kind})
		}
		excluded := make([]string, 0, len(disliked))
		for id := range disliked {
			excluded = append(excluded, id)
		}
		in["task"], in["request"] = "adapt", "Adapt the previous plan to the learner's feedback."
		in["previous_plan"], in["feedback"], in["excluded_ids"] = days, feedback, excluded
	}
	raw, _ := json.Marshal(in)
	return string(raw)
}

// signalIDs are the default taste signals: favorites plus anything the learner liked.
func (p *planner) signalIDs() []string {
	ids := make([]string, 0, len(p.learner.Favorites))
	for _, f := range p.learner.Favorites {
		ids = append(ids, f.ID)
	}
	if p.adapt != nil {
		for _, f := range p.adapt.feedback {
			if f.Kind == "like" {
				ids = append(ids, f.EntityID)
			}
		}
	}
	return ids
}

func (p *planner) remember(list []domain.Entity) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		p.seen[e.ID] = e
		out = append(out, compact(e))
	}
	return out
}

// compact keeps tool output small: the model needs ids, names, tags and a short description.
func compact(e domain.Entity) map[string]any {
	desc := e.Description
	if len(desc) > 240 {
		desc = desc[:240] + "…"
	}
	tags := e.Tags
	if len(tags) > 8 {
		tags = tags[:8]
	}
	return map[string]any{"id": e.ID, "name": e.Name, "type": e.Type, "year": e.Year, "description": desc, "tags": tags, "affinity": e.Affinity}
}

var typeEnum = []string{"movie", "tv_show", "book", "podcast", "artist"}

func (p *planner) tools() []Tool {
	planProps := map[string]any{
		"themes":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"summary": map[string]any{"type": "string"},
		"days": map[string]any{"type": "array", "items": obj(map[string]any{
			"day":       map[string]any{"type": "integer"},
			"entity_id": map[string]any{"type": "string"},
			"activity":  map[string]any{"type": "string"},
			"focus":     map[string]any{"type": "string", "enum": []string{"vocabulary", "listening", "reading", "speaking", "writing"}},
			"minutes":   map[string]any{"type": "integer"},
			"why":       map[string]any{"type": "string"},
		}, "day", "entity_id", "activity", "focus", "minutes", "why")},
	}
	final := Tool{Terminal: true, Run: p.savePlan, Spec: ToolSpec{Name: "build_plan", Description: "Save the final 7-day plan. Every entity_id must come from earlier tool results.", Parameters: obj(planProps, "themes", "summary", "days")}}
	if p.adapt != nil {
		planProps["changes"] = map[string]any{"type": "string"}
		final.Spec = ToolSpec{Name: "adapt_plan", Description: "Save the adapted 7-day plan and explain what changed. Every entity_id must come from the previous plan or earlier tool results.", Parameters: obj(planProps, "themes", "summary", "days", "changes")}
	}
	return []Tool{
		{
			Spec: ToolSpec{Name: "search_entities", Description: "Find taste-graph entities (with ids) by name.", Parameters: obj(map[string]any{
				"query": map[string]any{"type": "string"},
				"type":  map[string]any{"type": "string", "enum": typeEnum},
			}, "query", "type")},
			Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					Query string            `json:"query"`
					Type  domain.EntityType `json:"type"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || !domain.ValidEntityType(in.Type) || strings.TrimSpace(in.Query) == "" {
					return nil, errors.New("query and a supported type are required")
				}
				found, err := p.svc.Taste.Search(ctx, in.Query, in.Type, 5)
				if err != nil {
					return nil, err
				}
				return map[string]any{"entities": p.remember(found)}, nil
			},
		},
		{
			Spec: ToolSpec{Name: "get_tags", Description: "Look up taste-graph tags (genres, themes) by keyword; tag ids can steer get_taste_insights.", Parameters: obj(map[string]any{
				"query": map[string]any{"type": "string"},
			}, "query")},
			Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || strings.TrimSpace(in.Query) == "" {
					return nil, errors.New("query is required")
				}
				tags, err := p.svc.Taste.Tags(ctx, in.Query, 10)
				if err != nil {
					return nil, err
				}
				return map[string]any{"tags": tags}, nil
			},
		},
		{
			Spec: ToolSpec{Name: "get_taste_insights", Description: "Get entities of one type that people with the learner's taste also like. Defaults to the learner's favorites (and liked items) as signals.", Parameters: obj(map[string]any{
				"type":       map[string]any{"type": "string", "enum": typeEnum},
				"entity_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"tag_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"take":       map[string]any{"type": "integer"},
			}, "type")},
			Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var in struct {
					Type      domain.EntityType `json:"type"`
					EntityIDs []string          `json:"entity_ids"`
					TagIDs    []string          `json:"tag_ids"`
					Take      int               `json:"take"`
				}
				if err := json.Unmarshal(raw, &in); err != nil || !domain.ValidEntityType(in.Type) {
					return nil, errors.New("a supported type is required")
				}
				if len(in.EntityIDs) == 0 {
					in.EntityIDs = p.signalIDs()
				}
				if in.Take <= 0 || in.Take > 10 {
					in.Take = 8
				}
				found, err := p.svc.Taste.Insights(ctx, InsightsQuery{Type: in.Type, EntityIDs: in.EntityIDs, TagIDs: in.TagIDs, Take: in.Take})
				if err != nil {
					return nil, err
				}
				out := map[string]any{"entities": p.remember(found)}
				if len(found) == 0 {
					// Qloo silently ignores invalid parameters, so an empty result may mean a bad id.
					out["warning"] = "no results; an id or parameter may be invalid"
				}
				return out, nil
			},
		},
		final,
	}
}

func obj(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func (p *planner) savePlan(_ context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Themes  []string `json:"themes"`
		Summary string   `json:"summary"`
		Changes string   `json:"changes"`
		Days    []struct {
			Day      int    `json:"day"`
			EntityID string `json:"entity_id"`
			Activity string `json:"activity"`
			Focus    string `json:"focus"`
			Minutes  int    `json:"minutes"`
			Why      string `json:"why"`
		} `json:"days"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, errors.New("arguments do not match the plan schema")
	}
	if len(in.Days) != 7 {
		return nil, fmt.Errorf("plan needs exactly 7 days, got %d", len(in.Days))
	}
	if p.adapt != nil && strings.TrimSpace(in.Changes) == "" {
		return nil, errors.New("explain the changes")
	}
	favorites := map[string]bool{}
	for _, f := range p.learner.Favorites {
		favorites[f.ID] = true
	}
	disliked := map[string]bool{}
	if p.adapt != nil {
		disliked = p.adapt.disliked()
	}
	days, types := make([]domain.PlanDay, 7), map[domain.EntityType]bool{}
	for _, d := range in.Days {
		if d.Day < 1 || d.Day > 7 || days[d.Day-1].Day != 0 {
			return nil, fmt.Errorf("day %d is out of range or repeated", d.Day)
		}
		if disliked[d.EntityID] {
			return nil, fmt.Errorf("day %d: the learner disliked this item", d.Day)
		}
		e, ok := p.seen[d.EntityID]
		if !ok {
			return nil, fmt.Errorf("day %d: entity_id %q was not returned by any tool in this run", d.Day, d.EntityID)
		}
		if favorites[d.EntityID] {
			return nil, fmt.Errorf("day %d: do not schedule the learner's own favorite", d.Day)
		}
		if d.Minutes < 5 || d.Minutes > p.learner.DailyMinutes {
			return nil, fmt.Errorf("day %d: minutes must be 5-%d", d.Day, p.learner.DailyMinutes)
		}
		if strings.TrimSpace(d.Activity) == "" || strings.TrimSpace(d.Why) == "" {
			return nil, fmt.Errorf("day %d: activity and why are required", d.Day)
		}
		types[e.Type] = true
		days[d.Day-1] = domain.PlanDay{Day: d.Day, Entity: e, Activity: d.Activity, Focus: d.Focus, Minutes: d.Minutes, Why: d.Why}
	}
	if len(types) < 2 {
		return nil, errors.New("mix at least two content types")
	}
	p.plan.Themes, p.plan.Summary, p.plan.Days, p.plan.Changes = in.Themes, in.Summary, days, in.Changes
	p.built = true
	return map[string]any{"saved": true, "plan_id": p.plan.ID}, nil
}
