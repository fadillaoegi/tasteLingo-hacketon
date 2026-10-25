package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"tastelingo/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

type Repository struct{ Pool *pgxpool.Pool }

// Open connects and applies the idempotent schema.
func Open(ctx context.Context, url string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err == nil {
		_, err = pool.Exec(ctx, schema)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &Repository{Pool: pool}, nil
}

func (r *Repository) CreateLearner(ctx context.Context, l domain.Learner) error {
	favorites, _ := json.Marshal(l.Favorites)
	_, err := r.Pool.Exec(ctx, `INSERT INTO learners (id, name, level, goal, daily_minutes, favorites, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		l.ID, l.Name, l.Level, l.Goal, l.DailyMinutes, favorites, l.CreatedAt)
	return err
}

func (r *Repository) GetLearner(ctx context.Context, id string) (domain.Learner, error) {
	var l domain.Learner
	var favorites []byte
	err := r.Pool.QueryRow(ctx, `SELECT id, name, level, goal, daily_minutes, favorites, created_at FROM learners WHERE id=$1`, id).
		Scan(&l.ID, &l.Name, &l.Level, &l.Goal, &l.DailyMinutes, &favorites, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, domain.ErrNotFound
	}
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(favorites, &l.Favorites)
}

func (r *Repository) SaveRun(ctx context.Context, run domain.AgentRun) error {
	steps, _ := json.Marshal(run.Steps)
	_, err := r.Pool.Exec(ctx, `INSERT INTO agent_runs (id, learner_id, task, status, taste_mode, llm_mode, steps, error, started_at, finished_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, steps=EXCLUDED.steps, error=EXCLUDED.error, finished_at=EXCLUDED.finished_at`,
		run.ID, run.LearnerID, run.Task, run.Status, run.TasteMode, run.LLMMode, steps, run.Error, run.StartedAt, run.FinishedAt)
	return err
}

func (r *Repository) GetRun(ctx context.Context, id string) (domain.AgentRun, error) {
	var run domain.AgentRun
	var steps []byte
	err := r.Pool.QueryRow(ctx, `SELECT id, learner_id, task, status, taste_mode, llm_mode, steps, error, started_at, finished_at FROM agent_runs WHERE id=$1`, id).
		Scan(&run.ID, &run.LearnerID, &run.Task, &run.Status, &run.TasteMode, &run.LLMMode, &steps, &run.Error, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, domain.ErrNotFound
	}
	if err != nil {
		return run, err
	}
	return run, json.Unmarshal(steps, &run.Steps)
}

func (r *Repository) SavePlan(ctx context.Context, p domain.Plan) error {
	themes, _ := json.Marshal(p.Themes)
	days, _ := json.Marshal(p.Days)
	_, err := r.Pool.Exec(ctx, `INSERT INTO plans (id, learner_id, run_id, version, grounding, themes, summary, days, changes, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		p.ID, p.LearnerID, p.RunID, p.Version, p.Grounding, themes, p.Summary, days, p.Changes, p.CreatedAt)
	return err
}

const planColumns = `id, learner_id, run_id, version, grounding, themes, summary, days, changes, created_at`

func scanPlan(row pgx.Row) (domain.Plan, error) {
	var p domain.Plan
	var themes, days []byte
	err := row.Scan(&p.ID, &p.LearnerID, &p.RunID, &p.Version, &p.Grounding, &themes, &p.Summary, &days, &p.Changes, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(themes, &p.Themes); err != nil {
		return p, err
	}
	return p, json.Unmarshal(days, &p.Days)
}

func (r *Repository) LatestPlan(ctx context.Context, learnerID, grounding string) (domain.Plan, error) {
	return scanPlan(r.Pool.QueryRow(ctx, `SELECT `+planColumns+` FROM plans WHERE learner_id=$1 AND grounding=$2 ORDER BY version DESC LIMIT 1`, learnerID, grounding))
}

func (r *Repository) GetPlan(ctx context.Context, id string) (domain.Plan, error) {
	return scanPlan(r.Pool.QueryRow(ctx, `SELECT `+planColumns+` FROM plans WHERE id=$1`, id))
}

// SaveExercise keeps the first exercise of a plan day when two requests race.
func (r *Repository) SaveExercise(ctx context.Context, ex domain.Exercise) error {
	vocabulary, _ := json.Marshal(ex.Vocabulary)
	questions, _ := json.Marshal(ex.Questions)
	prompt, _ := json.Marshal(ex.Prompt)
	_, err := r.Pool.Exec(ctx, `INSERT INTO exercises (id, plan_id, day, entity_id, run_id, vocabulary, questions, prompt, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (plan_id, day) DO NOTHING`,
		ex.ID, ex.PlanID, ex.Day, ex.EntityID, ex.RunID, vocabulary, questions, prompt, ex.CreatedAt)
	return err
}

const exerciseColumns = `id, plan_id, day, entity_id, run_id, vocabulary, questions, prompt, created_at`

func scanExercise(row pgx.Row) (domain.Exercise, error) {
	var ex domain.Exercise
	var vocabulary, questions, prompt []byte
	err := row.Scan(&ex.ID, &ex.PlanID, &ex.Day, &ex.EntityID, &ex.RunID, &vocabulary, &questions, &prompt, &ex.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ex, domain.ErrNotFound
	}
	if err != nil {
		return ex, err
	}
	return ex, errors.Join(json.Unmarshal(vocabulary, &ex.Vocabulary), json.Unmarshal(questions, &ex.Questions), json.Unmarshal(prompt, &ex.Prompt))
}

func (r *Repository) GetExercise(ctx context.Context, id string) (domain.Exercise, error) {
	return scanExercise(r.Pool.QueryRow(ctx, `SELECT `+exerciseColumns+` FROM exercises WHERE id=$1`, id))
}

func (r *Repository) FindExercise(ctx context.Context, planID string, day int) (domain.Exercise, error) {
	return scanExercise(r.Pool.QueryRow(ctx, `SELECT `+exerciseColumns+` FROM exercises WHERE plan_id=$1 AND day=$2`, planID, day))
}

func (r *Repository) SaveAttempt(ctx context.Context, a domain.Attempt) error {
	answers, _ := json.Marshal(a.Answers)
	correct, _ := json.Marshal(a.Correct)
	_, err := r.Pool.Exec(ctx, `INSERT INTO attempts (id, exercise_id, answers, correct, score, response, feedback, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		a.ID, a.ExerciseID, answers, correct, a.Score, a.Response, a.Feedback, a.CreatedAt)
	return err
}

func (r *Repository) AddFeedback(ctx context.Context, f domain.Feedback) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO feedback (id, learner_id, plan_id, day, entity_id, entity_name, kind, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		f.ID, f.LearnerID, f.PlanID, f.Day, f.EntityID, f.EntityName, f.Kind, f.CreatedAt)
	return err
}

func (r *Repository) PlanFeedback(ctx context.Context, planID string) ([]domain.Feedback, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, learner_id, plan_id, day, entity_id, entity_name, kind, created_at FROM feedback WHERE plan_id=$1 ORDER BY created_at, id`, planID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Feedback, error) {
		var f domain.Feedback
		return f, row.Scan(&f.ID, &f.LearnerID, &f.PlanID, &f.Day, &f.EntityID, &f.EntityName, &f.Kind, &f.CreatedAt)
	})
}
