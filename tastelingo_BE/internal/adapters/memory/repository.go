package memory

import (
	"context"
	"sync"
	"tastelingo/internal/domain"
)

// Repository keeps data in process memory for tests and quick local runs.
type Repository struct {
	mu        sync.Mutex
	learners  map[string]domain.Learner
	runs      map[string]domain.AgentRun
	plans     []domain.Plan
	exercises []domain.Exercise
	attempts  []domain.Attempt
	feedback  []domain.Feedback
}

func New() *Repository {
	return &Repository{learners: map[string]domain.Learner{}, runs: map[string]domain.AgentRun{}}
}

func (r *Repository) CreateLearner(_ context.Context, l domain.Learner) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.learners[l.ID] = l
	return nil
}

func (r *Repository) GetLearner(_ context.Context, id string) (domain.Learner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.learners[id]
	if !ok {
		return domain.Learner{}, domain.ErrNotFound
	}
	return l, nil
}

func (r *Repository) SaveRun(_ context.Context, run domain.AgentRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	run.Steps = append([]domain.TraceStep(nil), run.Steps...)
	r.runs[run.ID] = run
	return nil
}

func (r *Repository) GetRun(_ context.Context, id string) (domain.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return domain.AgentRun{}, domain.ErrNotFound
	}
	return run, nil
}

func (r *Repository) SavePlan(_ context.Context, p domain.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans = append(r.plans, p)
	return nil
}

func (r *Repository) LatestPlan(_ context.Context, learnerID, grounding string) (domain.Plan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.plans) - 1; i >= 0; i-- {
		if p := r.plans[i]; p.LearnerID == learnerID && p.Grounding == grounding {
			return p, nil
		}
	}
	return domain.Plan{}, domain.ErrNotFound
}

func (r *Repository) GetPlan(_ context.Context, id string) (domain.Plan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.plans {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Plan{}, domain.ErrNotFound
}

// SaveExercise keeps the first exercise of a plan day, like the unique index in PostgreSQL.
func (r *Repository) SaveExercise(_ context.Context, ex domain.Exercise) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.exercises {
		if e.PlanID == ex.PlanID && e.Day == ex.Day {
			return nil
		}
	}
	r.exercises = append(r.exercises, ex)
	return nil
}

func (r *Repository) GetExercise(_ context.Context, id string) (domain.Exercise, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.exercises {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.Exercise{}, domain.ErrNotFound
}

func (r *Repository) FindExercise(_ context.Context, planID string, day int) (domain.Exercise, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.exercises {
		if e.PlanID == planID && e.Day == day {
			return e, nil
		}
	}
	return domain.Exercise{}, domain.ErrNotFound
}

func (r *Repository) SaveAttempt(_ context.Context, a domain.Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts = append(r.attempts, a)
	return nil
}

func (r *Repository) AddFeedback(_ context.Context, f domain.Feedback) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feedback = append(r.feedback, f)
	return nil
}

func (r *Repository) PlanFeedback(_ context.Context, planID string) ([]domain.Feedback, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Feedback{}
	for _, f := range r.feedback {
		if f.PlanID == planID {
			out = append(out, f)
		}
	}
	return out, nil
}
