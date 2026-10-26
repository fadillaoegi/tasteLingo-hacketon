package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
	"time"
)

type Handler struct {
	Svc *application.Service
	// Agent limits requests that run the LLM agent; Light limits Qloo search and
	// other writes. Nil disables limiting (local development and tests).
	Agent, Light *Limiter
	// StaticDir, when set, serves the built frontend from the same origin.
	StaticDir string
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	agent, light := h.Agent.Wrap, h.Light.Wrap
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/search", light(h.search))
	mux.HandleFunc("POST /api/learners", light(h.createLearner))
	mux.HandleFunc("POST /api/learners/{id}/plan", agent(h.buildPlan))
	mux.HandleFunc("GET /api/learners/{id}/plan", h.latestPlan)
	mux.HandleFunc("GET /api/learners/{id}/session", h.session)
	mux.HandleFunc("GET /api/runs/{id}", h.run)
	mux.HandleFunc("POST /api/demo", light(h.demo))
	mux.HandleFunc("POST /api/learners/{id}/compare", agent(h.buildComparison))
	mux.HandleFunc("GET /api/learners/{id}/compare", h.latestComparison)
	mux.HandleFunc("POST /api/learners/{id}/feedback", light(h.feedback))
	mux.HandleFunc("POST /api/learners/{id}/adapt", agent(h.adapt))
	mux.HandleFunc("GET /api/plans/{id}/feedback", h.planFeedback)
	mux.HandleFunc("POST /api/plans/{id}/days/{day}/exercise", agent(h.exercise))
	mux.HandleFunc("POST /api/exercises/{id}/attempts", agent(h.attempt))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { fail(w, domain.ErrNotFound) })
	if h.StaticDir != "" {
		mux.Handle("/", spa(h.StaticDir))
	}
	return secure(mux)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"status": "ok", "modes": h.Svc.Modes()})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	found, err := h.Svc.Search(r.Context(), r.URL.Query().Get("q"), domain.EntityType(r.URL.Query().Get("type")))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"entities": found, "source": h.Svc.Taste.Mode()})
}

func (h *Handler) createLearner(w http.ResponseWriter, r *http.Request) {
	var in domain.Learner
	if !decode(w, r, &in) {
		return
	}
	l, err := h.Svc.CreateLearner(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, l)
}

func (h *Handler) buildPlan(w http.ResponseWriter, r *http.Request) {
	h.agentPlan(w, r, h.Svc.BuildPlan)
}

func (h *Handler) buildComparison(w http.ResponseWriter, r *http.Request) {
	h.agentPlan(w, r, h.Svc.BuildComparison)
}

func (h *Handler) adapt(w http.ResponseWriter, r *http.Request) {
	h.agentPlan(w, r, h.Svc.AdaptPlan)
}

// agentPlan runs a planning use case; a failed run is returned so the UI can show its trace.
func (h *Handler) agentPlan(w http.ResponseWriter, r *http.Request, use func(context.Context, string) (domain.Plan, domain.AgentRun, error)) {
	ctx, cancel := context.WithTimeout(r.Context(), agentTimeout)
	defer cancel()
	plan, run, err := use(ctx, r.PathValue("id"))
	if err != nil && run.ID == "" {
		fail(w, err)
		return
	}
	if err != nil {
		slog.Warn("planning run failed", "run", run.ID, "error", err)
		write(w, agentFailed, map[string]any{"error": "the agent could not finish", "run": run})
		return
	}
	write(w, http.StatusOK, map[string]any{"plan": plan, "run": run})
}

func (h *Handler) latestPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := h.Svc.LatestPlan(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, plan)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	session, err := h.Svc.Session(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, session)
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	run, err := h.Svc.Run(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, run)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		write(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		write(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		slog.Error("request failed", "error", err)
		write(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

const agentTimeout = 120 * time.Second

// agentFailed reports a run that ended without a valid result. It is not 502, because
// proxies such as Cloudflare replace 502 bodies and the client would lose the trace.
const agentFailed = http.StatusInternalServerError

func (h *Handler) latestComparison(w http.ResponseWriter, r *http.Request) {
	plan, err := h.Svc.LatestComparison(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, plan)
}

func (h *Handler) demo(w http.ResponseWriter, r *http.Request) {
	l, err := h.Svc.CreateDemoLearner(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, l)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(v); err != nil {
		fail(w, errors.Join(domain.ErrValidation, errors.New("invalid JSON body")))
		return false
	}
	return true
}

func (h *Handler) feedback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PlanID string `json:"plan_id"`
		Day    int    `json:"day"`
		Kind   string `json:"kind"`
	}
	if !decode(w, r, &in) {
		return
	}
	f, err := h.Svc.GiveFeedback(r.Context(), r.PathValue("id"), in.PlanID, in.Day, in.Kind)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, f)
}

func (h *Handler) planFeedback(w http.ResponseWriter, r *http.Request) {
	list, err := h.Svc.PlanFeedback(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"feedback": list})
}

func (h *Handler) exercise(w http.ResponseWriter, r *http.Request) {
	day, err := strconv.Atoi(r.PathValue("day"))
	if err != nil {
		fail(w, domain.ErrNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentTimeout)
	defer cancel()
	ex, run, err := h.Svc.Exercise(ctx, r.PathValue("id"), day)
	if err != nil && run != nil && run.ID != "" {
		slog.Warn("exercise run failed", "run", run.ID, "error", err)
		write(w, agentFailed, map[string]any{"error": "the agent could not finish", "run": run})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"exercise": ex.ForLearner(), "run": run})
}

func (h *Handler) attempt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Answers  []int  `json:"answers"`
		Response string `json:"response"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentTimeout)
	defer cancel()
	at, ex, err := h.Svc.Attempt(ctx, r.PathValue("id"), in.Answers, in.Response)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, map[string]any{"attempt": at, "exercise": ex})
}
