package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"tastelingo/internal/domain"
)

const exercisePrompt = `You are TasteLingo's exercise writer for English learners.
Write practice material for one item of the learner's plan, at their CEFR level.
Use ONLY the item's description, tags and metadata given to you. Do not quote or
reproduce lyrics, scripts, book text or any other copyrighted content.
- vocabulary: 6-10 useful words or phrases connected to the item's themes, each with a
  simple meaning and an original example sentence.
- questions: 3 multiple-choice questions (4 options, one correct, answer = 0-based index)
  about the description and metadata.
- prompt: one speaking or writing task linking the item to the learner's goal.
Call generate_exercise exactly once.`

const feedbackPrompt = `You are a supportive English tutor. Give feedback on the learner's answer
to the task in 2-4 short sentences at their CEFR level: one strength, then correct up to two
mistakes by quoting the phrase and giving a better version. Reply in plain English text.`

// Exercise returns the exercise of one plan day, generating it with the agent on first use.
func (s *Service) Exercise(ctx context.Context, planID string, day int) (domain.Exercise, *domain.AgentRun, error) {
	if ex, err := s.Repo.FindExercise(ctx, planID, day); err == nil {
		return ex, nil, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Exercise{}, nil, err
	}
	plan, err := s.Repo.GetPlan(ctx, planID)
	if err != nil {
		return domain.Exercise{}, nil, err
	}
	if plan.Grounding != domain.GroundingTasteGraph || day < 1 || day > len(plan.Days) {
		return domain.Exercise{}, nil, domain.ErrNotFound
	}
	learner, err := s.Repo.GetLearner(ctx, plan.LearnerID)
	if err != nil {
		return domain.Exercise{}, nil, err
	}
	item := plan.Days[day-1]
	run, err := s.startRun(ctx, plan.LearnerID, "generate_exercise")
	if err != nil {
		return domain.Exercise{}, nil, err
	}
	ex := domain.Exercise{ID: NewID(), PlanID: planID, Day: day, EntityID: item.Entity.ID, RunID: run.ID}
	built := false
	tool := Tool{Terminal: true, Spec: exerciseSpec, Redact: hideAnswers, Run: func(_ context.Context, raw json.RawMessage) (any, error) {
		if err := parseExercise(raw, &ex); err != nil {
			return nil, err
		}
		built = true
		return map[string]any{"saved": true}, nil
	}}
	task, _ := json.Marshal(map[string]any{
		"task":    "exercise",
		"learner": map[string]any{"level": learner.Level, "goal": learner.Goal},
		"item":    compact(item.Entity),
		"plan":    map[string]any{"activity": item.Activity, "focus": item.Focus},
	})
	runErr := s.agent().Run(ctx, &run, exercisePrompt, string(task), []Tool{tool})
	if runErr == nil && !built {
		runErr = ErrAgentIncomplete
	}
	saveCtx, cancel, err := s.finishRun(ctx, &run, runErr)
	defer cancel()
	if err != nil {
		return domain.Exercise{}, &run, err
	}
	if runErr != nil {
		return domain.Exercise{}, &run, runErr
	}
	ex.CreatedAt = *run.FinishedAt
	if err := s.Repo.SaveExercise(saveCtx, ex); err != nil {
		return domain.Exercise{}, &run, err
	}
	// A concurrent request may have stored this day's exercise first; return the stored one.
	stored, err := s.Repo.FindExercise(saveCtx, planID, day)
	return stored, &run, err
}

var exerciseSpec = ToolSpec{Name: "generate_exercise", Description: "Save the exercise for this plan item.", Parameters: obj(map[string]any{
	"vocabulary": map[string]any{"type": "array", "items": obj(map[string]any{
		"term": map[string]any{"type": "string"}, "meaning": map[string]any{"type": "string"}, "example": map[string]any{"type": "string"},
	}, "term", "meaning", "example")},
	"questions": map[string]any{"type": "array", "items": obj(map[string]any{
		"question": map[string]any{"type": "string"},
		"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"answer":   map[string]any{"type": "integer"},
	}, "question", "options", "answer")},
	"prompt": obj(map[string]any{
		"kind": map[string]any{"type": "string", "enum": []string{"speaking", "writing"}},
		"text": map[string]any{"type": "string"},
	}, "kind", "text"),
}, "vocabulary", "questions", "prompt")}

func parseExercise(raw json.RawMessage, ex *domain.Exercise) error {
	var in struct {
		Vocabulary []domain.VocabCard `json:"vocabulary"`
		Questions  []domain.Question  `json:"questions"`
		Prompt     domain.Prompt      `json:"prompt"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return errors.New("arguments do not match the generate_exercise schema")
	}
	if len(in.Vocabulary) < 5 || len(in.Vocabulary) > 10 {
		return errors.New("vocabulary needs 5-10 items")
	}
	for i, v := range in.Vocabulary {
		if strings.TrimSpace(v.Term) == "" || strings.TrimSpace(v.Meaning) == "" || strings.TrimSpace(v.Example) == "" {
			return fmt.Errorf("vocabulary %d needs term, meaning and example", i+1)
		}
	}
	if len(in.Questions) < 2 || len(in.Questions) > 5 {
		return errors.New("questions needs 2-5 items")
	}
	for i, q := range in.Questions {
		if strings.TrimSpace(q.Question) == "" || len(q.Options) < 3 || len(q.Options) > 4 || q.Answer == nil || *q.Answer < 0 || *q.Answer >= len(q.Options) {
			return fmt.Errorf("question %d needs text, 3-4 options and a valid answer index", i+1)
		}
	}
	if in.Prompt.Kind != "speaking" && in.Prompt.Kind != "writing" || strings.TrimSpace(in.Prompt.Text) == "" {
		return errors.New("prompt needs kind speaking or writing, and text")
	}
	ex.Vocabulary, ex.Questions, ex.Prompt = in.Vocabulary, in.Questions, in.Prompt
	return nil
}

// Attempt scores the multiple-choice answers and asks the LLM for short feedback
// on the free response. The answer key is returned with the result.
func (s *Service) Attempt(ctx context.Context, exerciseID string, answers []int, response string) (domain.Attempt, domain.Exercise, error) {
	ex, err := s.Repo.GetExercise(ctx, exerciseID)
	if err != nil {
		return domain.Attempt{}, domain.Exercise{}, err
	}
	response = strings.TrimSpace(response)
	if len(answers) != len(ex.Questions) || len(response) > 1500 {
		return domain.Attempt{}, ex, errors.Join(domain.ErrValidation, errors.New("answer every question; responses are limited to 1500 characters"))
	}
	at := domain.Attempt{ID: NewID(), ExerciseID: exerciseID, Answers: answers, Response: response, Correct: make([]bool, len(answers))}
	for i, a := range answers {
		at.Correct[i] = a == *ex.Questions[i].Answer
		if at.Correct[i] {
			at.Score++
		}
	}
	if response != "" {
		plan, err := s.Repo.GetPlan(ctx, ex.PlanID)
		if err != nil {
			return domain.Attempt{}, ex, err
		}
		learner, err := s.Repo.GetLearner(ctx, plan.LearnerID)
		if err != nil {
			return domain.Attempt{}, ex, err
		}
		run, err := s.startRun(ctx, plan.LearnerID, "attempt_feedback")
		if err != nil {
			return domain.Attempt{}, ex, err
		}
		task, _ := json.Marshal(map[string]any{"task": "attempt_feedback", "level": learner.Level, "prompt": ex.Prompt, "response": response})
		text, runErr := s.agent().Ask(ctx, &run, feedbackPrompt, string(task))
		saveCtx, cancel, err := s.finishRun(ctx, &run, runErr)
		defer cancel()
		if err != nil {
			return domain.Attempt{}, ex, err
		}
		ctx = saveCtx
		// The score still counts when feedback is unavailable.
		at.Feedback = strings.TrimSpace(text)
		if runErr != nil {
			at.Feedback = "Feedback is unavailable right now; your answers were still scored."
		}
	}
	at.CreatedAt = s.Now().UTC()
	return at, ex, s.Repo.SaveAttempt(ctx, at)
}

// GiveFeedback records how the learner felt about one day of their plan.
func (s *Service) GiveFeedback(ctx context.Context, learnerID, planID string, day int, kind string) (domain.Feedback, error) {
	f := domain.Feedback{ID: NewID(), LearnerID: learnerID, PlanID: planID, Day: day, Kind: kind, CreatedAt: s.Now().UTC()}
	if err := f.Validate(); err != nil {
		return f, err
	}
	plan, err := s.Repo.GetPlan(ctx, planID)
	if err != nil {
		return f, err
	}
	if plan.LearnerID != learnerID || plan.Grounding != domain.GroundingTasteGraph || day > len(plan.Days) {
		return f, domain.ErrNotFound
	}
	f.EntityID, f.EntityName = plan.Days[day-1].Entity.ID, plan.Days[day-1].Entity.Name
	return f, s.Repo.AddFeedback(ctx, f)
}

func (s *Service) PlanFeedback(ctx context.Context, planID string) ([]domain.Feedback, error) {
	return s.Repo.PlanFeedback(ctx, planID)
}

// hideAnswers keeps the answer key out of the agent trace, which learners can read.
func hideAnswers(raw json.RawMessage) json.RawMessage {
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return json.RawMessage(`{"redacted":true}`)
	}
	if qs, ok := in["questions"].([]any); ok {
		for _, q := range qs {
			if m, ok := q.(map[string]any); ok {
				delete(m, "answer")
			}
		}
	}
	out, _ := json.Marshal(in)
	return out
}
