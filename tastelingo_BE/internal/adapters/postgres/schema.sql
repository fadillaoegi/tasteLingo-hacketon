-- Idempotent schema applied at startup. Entity snapshots inside JSONB are a
-- private server-side cache; they are never exported to the public repository.
CREATE TABLE IF NOT EXISTS learners (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    level         TEXT NOT NULL,
    goal          TEXT NOT NULL,
    daily_minutes INT NOT NULL,
    favorites     JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_runs (
    id          TEXT PRIMARY KEY,
    learner_id  TEXT NOT NULL REFERENCES learners(id) ON DELETE CASCADE,
    task        TEXT NOT NULL,
    status      TEXT NOT NULL,
    taste_mode  TEXT NOT NULL,
    llm_mode    TEXT NOT NULL,
    steps       JSONB NOT NULL DEFAULT '[]',
    error       TEXT NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS plans (
    id         TEXT PRIMARY KEY,
    learner_id TEXT NOT NULL REFERENCES learners(id) ON DELETE CASCADE,
    run_id     TEXT NOT NULL REFERENCES agent_runs(id),
    version    INT NOT NULL,
    grounding  TEXT NOT NULL,
    themes     JSONB NOT NULL,
    summary    TEXT NOT NULL,
    days       JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (learner_id, grounding, version)
);

ALTER TABLE plans ADD COLUMN IF NOT EXISTS changes TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS exercises (
    id         TEXT PRIMARY KEY,
    plan_id    TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    day        INT NOT NULL,
    entity_id  TEXT NOT NULL,
    run_id     TEXT NOT NULL REFERENCES agent_runs(id),
    vocabulary JSONB NOT NULL,
    questions  JSONB NOT NULL,
    prompt     JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (plan_id, day)
);

CREATE TABLE IF NOT EXISTS attempts (
    id          TEXT PRIMARY KEY,
    exercise_id TEXT NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    answers     JSONB NOT NULL,
    correct     JSONB NOT NULL,
    score       INT NOT NULL,
    response    TEXT NOT NULL,
    feedback    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS feedback (
    id          TEXT PRIMARY KEY,
    learner_id  TEXT NOT NULL REFERENCES learners(id) ON DELETE CASCADE,
    plan_id     TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    day         INT NOT NULL,
    entity_id   TEXT NOT NULL,
    entity_name TEXT NOT NULL,
    kind        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS feedback_plan_idx ON feedback (plan_id, created_at);
