package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"tastelingo/internal/domain"
	"time"
)

type Service struct {
	Repo  Repository
	Taste TasteGraph
	LLM   LLM
	Now   func() time.Time
}

func NewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) Modes() map[string]string {
	return map[string]string{"taste_graph": s.Taste.Mode(), "llm": s.LLM.Mode()}
}

func (s *Service) agent() Agent { return Agent{LLM: s.LLM, MaxTurns: 10, Now: s.Now} }

func (s *Service) Search(ctx context.Context, query string, t domain.EntityType) ([]domain.Entity, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 || len(query) > 80 {
		return nil, errors.Join(domain.ErrValidation, errors.New("query must be 2-80 characters"))
	}
	if t != "" && !domain.ValidEntityType(t) {
		return nil, errors.Join(domain.ErrValidation, errors.New("unsupported type"))
	}
	return s.Taste.Search(ctx, query, t, 8)
}

func (s *Service) CreateLearner(ctx context.Context, l domain.Learner) (domain.Learner, error) {
	l.Name = strings.TrimSpace(l.Name)
	if err := l.Validate(); err != nil {
		return domain.Learner{}, err
	}
	l.ID, l.CreatedAt = NewID(), s.Now().UTC()
	for i := range l.Favorites {
		l.Favorites[i].Source = domain.SourceUser
	}
	return l, s.Repo.CreateLearner(ctx, l)
}

// demoFavorites mirror the demo persona: a B1 learner preparing for job interviews.
var demoFavorites = []struct {
	query string
	t     domain.EntityType
}{{"Spirited Away", domain.Movie}, {"Hindia", domain.Artist}, {"The Mel Robbins Podcast", domain.Podcast}}

// CreateDemoLearner lets judges try the app without typing; favorites are
// resolved through the taste graph like a real onboarding.
func (s *Service) CreateDemoLearner(ctx context.Context) (domain.Learner, error) {
	l := domain.Learner{Name: "Rina", Level: "B1", Goal: "work", DailyMinutes: 30}
	for _, f := range demoFavorites {
		found, err := s.Taste.Search(ctx, f.query, f.t, 1)
		if err != nil {
			return domain.Learner{}, err
		}
		if len(found) > 0 {
			l.Favorites = append(l.Favorites, found[0])
		}
	}
	return s.CreateLearner(ctx, l)
}

func (s *Service) Run(ctx context.Context, id string) (domain.AgentRun, error) {
	return s.Repo.GetRun(ctx, id)
}

// startRun stores the run before the agent starts so an interrupted run still has a trace.
func (s *Service) startRun(ctx context.Context, learnerID, task string) (domain.AgentRun, error) {
	run := domain.AgentRun{ID: NewID(), LearnerID: learnerID, Task: task, Status: domain.RunRunning, TasteMode: s.Taste.Mode(), LLMMode: s.LLM.Mode(), StartedAt: s.Now().UTC()}
	return run, s.Repo.SaveRun(ctx, run)
}

// finishRun records the outcome. It saves even when the request context was
// cancelled, and returns a context for storing the run's results.
func (s *Service) finishRun(ctx context.Context, run *domain.AgentRun, runErr error) (context.Context, context.CancelFunc, error) {
	finished := s.Now().UTC()
	run.FinishedAt, run.Status = &finished, domain.RunSucceeded
	if runErr != nil {
		run.Status, run.Error = domain.RunFailed, runErr.Error()
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	return saveCtx, cancel, s.Repo.SaveRun(saveCtx, *run)
}

func (s *Service) nextVersion(ctx context.Context, learnerID, grounding string) (int, error) {
	prev, err := s.Repo.LatestPlan(ctx, learnerID, grounding)
	if errors.Is(err, domain.ErrNotFound) {
		return 1, nil
	}
	return prev.Version + 1, err
}
