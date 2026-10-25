package application

import (
	"context"
	"errors"
	"tastelingo/internal/domain"
)

// LearnerSession treats an unfinished plan and an expired browser profile as
// normal restore states rather than failed requests.
type LearnerSession struct {
	Learner  *domain.Learner   `json:"learner"`
	Plan     *domain.Plan      `json:"plan"`
	Run      *domain.AgentRun  `json:"run"`
	Feedback []domain.Feedback `json:"feedback"`
	Modes    map[string]string `json:"modes"`
}

func (s *Service) Session(ctx context.Context, id string) (LearnerSession, error) {
	out := LearnerSession{Feedback: []domain.Feedback{}, Modes: s.Modes()}
	l, err := s.Repo.GetLearner(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Learner = &l
	p, err := s.LatestPlan(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	run, err := s.Run(ctx, p.RunID)
	if err != nil {
		return out, err
	}
	feedback, err := s.Repo.PlanFeedback(ctx, p.ID)
	if err != nil {
		return out, err
	}
	out.Plan, out.Run = &p, &run
	if feedback != nil {
		out.Feedback = feedback
	}
	return out, nil
}
