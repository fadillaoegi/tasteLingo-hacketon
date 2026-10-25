package domain

import (
	"errors"
	"time"
)

type VocabCard struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
	Example string `json:"example"`
}

type Question struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	// Answer is hidden from the learner until an attempt is scored.
	Answer *int `json:"answer,omitempty"`
}

type Prompt struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Exercise is generated from an entity's description, tags and metadata only,
// never from the copyrighted work itself.
type Exercise struct {
	ID         string      `json:"id"`
	PlanID     string      `json:"plan_id"`
	Day        int         `json:"day"`
	EntityID   string      `json:"entity_id"`
	RunID      string      `json:"run_id"`
	Vocabulary []VocabCard `json:"vocabulary"`
	Questions  []Question  `json:"questions"`
	Prompt     Prompt      `json:"prompt"`
	CreatedAt  time.Time   `json:"created_at"`
}

// ForLearner returns a copy without the answer key.
func (e Exercise) ForLearner() Exercise {
	qs := make([]Question, len(e.Questions))
	for i, q := range e.Questions {
		q.Answer = nil
		qs[i] = q
	}
	e.Questions = qs
	return e
}

type Attempt struct {
	ID         string    `json:"id"`
	ExerciseID string    `json:"exercise_id"`
	Answers    []int     `json:"answers"`
	Correct    []bool    `json:"correct"`
	Score      int       `json:"score"`
	Response   string    `json:"response"`
	Feedback   string    `json:"feedback"`
	CreatedAt  time.Time `json:"created_at"`
}

// Feedback kinds a learner can give on one plan day.
var FeedbackKinds = []string{"too_hard", "too_easy", "dislike", "like"}

type Feedback struct {
	ID         string    `json:"id"`
	LearnerID  string    `json:"learner_id"`
	PlanID     string    `json:"plan_id"`
	Day        int       `json:"day"`
	EntityID   string    `json:"entity_id"`
	EntityName string    `json:"entity_name"`
	Kind       string    `json:"kind"`
	CreatedAt  time.Time `json:"created_at"`
}

func (f Feedback) Validate() error {
	if !contains(FeedbackKinds, f.Kind) {
		return errors.Join(ErrValidation, errors.New("kind must be too_hard, too_easy, dislike or like"))
	}
	if f.Day < 1 || f.Day > 7 {
		return errors.Join(ErrValidation, errors.New("day must be 1-7"))
	}
	return nil
}
