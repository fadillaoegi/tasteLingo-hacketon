package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"tastelingo/internal/adapters/httpapi"
	"tastelingo/internal/adapters/llm"
	"tastelingo/internal/adapters/memory"
	"tastelingo/internal/adapters/postgres"
	"tastelingo/internal/adapters/qloo"
	"tastelingo/internal/application"
	"time"
)

// load reads KEY=VALUE lines without overriding the real environment.
func load(path string) error {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid environment file")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		if _, ok := os.LookupEnv(key); !ok {
			if e = os.Setenv(key, value); e != nil {
				return e
			}
		}
	}
	return sc.Err()
}

func env(k, d string) string {
	if s, ok := os.LookupEnv(k); ok && s != "" {
		return s
	}
	return d
}

func run() error {
	path := flag.String("env", "../.env", "local environment configuration")
	flag.Parse()
	if e := load(*path); e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	svc := &application.Service{Now: time.Now}
	switch env("STORE_MODE", "postgres") {
	case "postgres":
		connect, done := context.WithTimeout(ctx, 8*time.Second)
		defer done()
		pg, e := postgres.Open(connect, os.Getenv("DATABASE_URL"))
		if e != nil {
			return fmt.Errorf("PostgreSQL connection failed (check DATABASE_URL and database availability)")
		}
		defer pg.Pool.Close()
		svc.Repo = pg
	case "memory":
		svc.Repo = memory.New()
		slog.Warn("Ephemeral memory storage enabled; data is lost on restart")
	default:
		return fmt.Errorf("invalid STORE_MODE")
	}
	switch env("QLOO_MODE", "mock") {
	case "live":
		if os.Getenv("QLOO_API_KEY") == "" {
			return fmt.Errorf("QLOO_MODE=live needs QLOO_API_KEY in the local environment")
		}
		svc.Taste = &qloo.Client{BaseURL: env("QLOO_BASE_URL", "https://hackathon.api.qloo.com"), Key: os.Getenv("QLOO_API_KEY"), HTTP: &http.Client{Timeout: 15 * time.Second}}
	case "mock":
		svc.Taste = qloo.Mock{}
		slog.Warn("QLOO_MODE=mock: taste data is synthetic, not from Qloo")
	default:
		return fmt.Errorf("invalid QLOO_MODE")
	}
	switch env("LLM_MODE", "mock") {
	case "gemini":
		model := env("GEMINI_MODEL", llm.DefaultGeminiModel)
		fallbackCSV, configured := os.LookupEnv("GEMINI_FALLBACK_MODELS")
		if !configured {
			fallbackCSV = llm.DefaultGeminiFallbackModels
		}
		fallbacks, err := llm.ParseGeminiFallbackModels(model, fallbackCSV)
		if err != nil {
			return fmt.Errorf("invalid Gemini model configuration: %w", err)
		}
		if os.Getenv("GEMINI_API_KEY") == "" {
			return fmt.Errorf("LLM_MODE=gemini needs GEMINI_API_KEY")
		}
		autoDiscover, err := strconv.ParseBool(env("GEMINI_AUTO_DISCOVER", "true"))
		if err != nil {
			return fmt.Errorf("GEMINI_AUTO_DISCOVER must be true or false")
		}
		svc.LLM = &llm.Gemini{Key: os.Getenv("GEMINI_API_KEY"), Model: model, FallbackModels: fallbacks, AutoDiscover: autoDiscover, HTTP: &http.Client{Timeout: 20 * time.Second}}
		slog.Info("Gemini model fallback configured", "primary", model, "fallbacks", fallbacks)
	case "mock":
		svc.LLM = llm.Mock{}
		slog.Warn("LLM_MODE=mock: plans come from a scripted planner")
	default:
		return fmt.Errorf("invalid LLM_MODE")
	}

	addr := env("API_ADDR", "127.0.0.1:8104")
	// Hosting platforms such as Render provide the port to listen on.
	if port := os.Getenv("PORT"); port != "" && os.Getenv("API_ADDR") == "" {
		addr = "0.0.0.0:" + port
	}
	handler := &httpapi.Handler{Svc: svc, StaticDir: os.Getenv("STATIC_DIR")}
	var e error
	if handler.Agent, handler.Light, e = limiters(); e != nil {
		return e
	}
	srv := &http.Server{Addr: addr, Handler: handler.Routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("TasteLingo API listening", "addr", addr, "modes", svc.Modes())
	if e := srv.ListenAndServe(); !errors.Is(e, http.ErrServerClosed) {
		return e
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

// limiters reads hourly request limits. RATE_LIMIT=false disables them for local use.
func limiters() (agent, light *httpapi.Limiter, err error) {
	enabled, err := strconv.ParseBool(env("RATE_LIMIT", "false"))
	if err != nil {
		return nil, nil, fmt.Errorf("RATE_LIMIT must be true or false")
	}
	if !enabled {
		return nil, nil, nil
	}
	trust, err := strconv.ParseBool(env("TRUST_PROXY", "false"))
	if err != nil {
		return nil, nil, fmt.Errorf("TRUST_PROXY must be true or false")
	}
	limit := func(key, def string) (int, error) {
		n, err := strconv.Atoi(env(key, def))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s must be a non-negative number", key)
		}
		return n, nil
	}
	var values [4]int
	for i, kv := range [][2]string{{"AGENT_LIMIT_PER_IP", "20"}, {"AGENT_LIMIT_GLOBAL", "300"}, {"LIGHT_LIMIT_PER_IP", "200"}, {"LIGHT_LIMIT_GLOBAL", "3000"}} {
		if values[i], err = limit(kv[0], kv[1]); err != nil {
			return nil, nil, err
		}
	}
	agent = &httpapi.Limiter{PerClient: values[0], Global: values[1], Window: time.Hour, TrustProxy: trust}
	light = &httpapi.Limiter{PerClient: values[2], Global: values[3], Window: time.Hour, TrustProxy: trust}
	slog.Info("rate limits enabled", "agent_per_ip_hour", values[0], "agent_global_hour", values[1], "trust_proxy", trust)
	return agent, light, nil
}
