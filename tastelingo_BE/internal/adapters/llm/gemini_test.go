package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"tastelingo/internal/application"
	"testing"
	"time"
)

type geminiTestTransport func(*http.Request) (*http.Response, error)

func (f geminiTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise real HTTP request/response handling without binding a local port.
func geminiTestClient(handler http.HandlerFunc) *http.Client {
	return &http.Client{Transport: geminiTestTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler(w, r)
		return w.Result(), nil
	})}
}

func TestGeminiParsesCallsAndEchoesRawTurn(t *testing.T) {
	var bodies []string
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "k" || !strings.HasSuffix(r.URL.Path, "/models/gemini-test:generateContent") {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[
			{"text":"hidden","thought":true},
			{"text":"Looking up insights."},
			{"functionCall":{"name":"get_tags","args":{"query":"anime"}},"thoughtSignature":"sig-1"}]}}]}`))
	})
	g := &Gemini{Key: "k", Model: "gemini-test", HTTP: client}
	tools := []application.ToolSpec{{Name: "get_tags", Description: "d", Parameters: map[string]any{"type": "object"}}}
	msg, err := g.Complete(context.Background(), "sys", []application.Message{{Role: application.RoleUser, Text: "hi"}}, tools)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "Looking up insights." || len(msg.Calls) != 1 || msg.Calls[0].Name != "get_tags" || !strings.HasPrefix(msg.Calls[0].ID, localID) {
		t.Fatalf("unexpected message %+v", msg)
	}
	history := []application.Message{{Role: application.RoleUser, Text: "hi"}, msg, {Role: application.RoleTool, Results: []application.ToolResult{{CallID: msg.Calls[0].ID, Name: "get_tags", Content: json.RawMessage(`{"tags":[]}`)}}}}
	if _, err := g.Complete(context.Background(), "sys", history, tools); err != nil {
		t.Fatal(err)
	}
	second := bodies[1]
	if !strings.Contains(second, `"thoughtSignature":"sig-1"`) {
		t.Fatalf("thought signature not echoed: %s", second)
	}
	if strings.Contains(second, localID) {
		t.Fatalf("adapter-invented call id sent to provider: %s", second)
	}
	if !strings.Contains(second, `"parametersJsonSchema"`) || !strings.Contains(second, `"functionResponse"`) {
		t.Fatalf("request shape unexpected: %s", second)
	}
}

func TestGeminiErrorHidesProviderMessage(t *testing.T) {
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"echo of private prompt"}}`))
	})
	g := &Gemini{Key: "k", Model: "gemini-test", HTTP: client}
	_, err := g.Complete(context.Background(), "sys", []application.Message{{Role: application.RoleUser, Text: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") || strings.Contains(err.Error(), "private") {
		t.Fatalf("unexpected error %v", err)
	}
}

const geminiTestOK = `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK"}]}}]}`

func TestGeminiFallbackCooldownAndRecovery(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	var paths []string
	var bodies []string
	primaryCalls := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if r.Header.Get("x-goog-api-key") != "secret" {
			t.Fatal("missing credential")
		}
		if strings.Contains(r.URL.Path, "gemini-primary:") {
			primaryCalls++
			if primaryCalls == 1 {
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(429)
				_, _ = io.WriteString(w, `{"error":{"status":"RESOURCE_EXHAUSTED"}}`)
				return
			}
		}
		_, _ = io.WriteString(w, geminiTestOK)
	})
	g := &Gemini{Key: "secret", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client, now: func() time.Time { return now }}
	history := []application.Message{{Role: application.RoleUser, Text: "Original learner context"}}
	tools := []application.ToolSpec{{Name: "get_tags", Parameters: map[string]any{"type": "object"}}}
	call := func(want string) {
		t.Helper()
		msg, err := g.Complete(context.Background(), "system", history, tools)
		if err != nil || msg.Model != want || msg.Text != "OK" {
			t.Fatalf("got %+v %v, want %s", msg, err, want)
		}
	}
	call("gemini-backup")
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatal("fallback changed original request")
	}
	now = now.Add(90 * time.Second)
	call("gemini-backup")
	if primaryCalls != 1 {
		t.Fatal("retried before Retry-After")
	}
	now = now.Add(31 * time.Second)
	call("gemini-primary")
	if len(paths) != 4 || primaryCalls != 2 {
		t.Fatalf("unexpected attempts %v", paths)
	}
}

func TestGeminiFallbackErrorClassification(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attempts := 0
			client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if attempts == 1 {
					w.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(w, geminiTestOK)
			})
			g := &Gemini{Key: "k", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
			_, err := g.Complete(context.Background(), "sys", nil, nil)
			shouldFallback := status == 404 || status == 429 || status >= 500
			if shouldFallback && (err != nil || attempts != 2) {
				t.Fatalf("fallback failed: attempts %d, %v", attempts, err)
			}
			if !shouldFallback && (err == nil || attempts != 1) {
				t.Fatalf("must stop: attempts %d, %v", attempts, err)
			}
		})
	}
}

func TestGeminiAllModelsLimitedAndCancellation(t *testing.T) {
	attempts := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"error":{"status":"RESOURCE_EXHAUSTED","message":"private prompt secret"}}`)
	})
	g := &Gemini{Key: "secret", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
	for range 2 {
		_, err := g.Complete(context.Background(), "sys", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "all configured models") || !strings.Contains(err.Error(), "gemini-backup") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if attempts != 2 {
		t.Fatalf("cooldown must prevent repeat attempts, got %d", attempts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Complete(ctx, "sys", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if attempts != 2 {
		t.Fatal("canceled request reached provider")
	}
}

func TestGeminiSwitchMidToolLoop(t *testing.T) {
	var paths []string
	var bodies []string
	primaryCalls := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if strings.Contains(r.URL.Path, "gemini-primary:") {
			primaryCalls++
			if primaryCalls == 1 {
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"private reasoning","thought":true},{"functionCall":{"name":"get_tags","args":{"query":"anime"}},"thoughtSignature":"primary-signature"}]}}]}`)
				return
			}
			w.WriteHeader(429)
			return
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"backup-call","name":"get_tags","args":{}},"thoughtSignature":"backup-signature"}]}}]}`)
	})
	g := &Gemini{Key: "k", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
	history := []application.Message{{Role: application.RoleUser, Text: "Find anime"}}
	first, err := g.Complete(context.Background(), "sys", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	history = append(history, first, application.Message{Role: application.RoleTool, Results: []application.ToolResult{{CallID: first.Calls[0].ID, Name: "get_tags", Content: json.RawMessage(`{"tags":["anime"]}`)}}})
	second, err := g.Complete(context.Background(), "sys", history, nil)
	if err != nil || second.Model != "gemini-backup" {
		t.Fatalf("failed to switch: %+v %v", second, err)
	}
	if !strings.Contains(bodies[1], "primary-signature") {
		t.Fatal("original model lost its signature")
	}
	foreign := bodies[2]
	if strings.Contains(foreign, "primary-signature") || strings.Contains(foreign, "private reasoning") || strings.Contains(foreign, localID) {
		t.Fatalf("foreign state leaked: %s", foreign)
	}
	if !strings.Contains(foreign, "skip_thought_signature_validator") || !strings.Contains(foreign, `"functionResponse"`) || !strings.Contains(foreign, `"anime"`) {
		t.Fatalf("tool context lost: %s", foreign)
	}
	// Even after recovery, continue the existing loop on the model that owns it.
	g.mu.Lock()
	delete(g.blocked, "gemini-primary")
	g.mu.Unlock()
	history = append(history, second, application.Message{Role: application.RoleTool, Results: []application.ToolResult{{CallID: "backup-call", Name: "get_tags", Content: json.RawMessage(`{"tags":[]}`)}}})
	if _, err := g.Complete(context.Background(), "sys", history, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(paths[3], "gemini-backup:") || !strings.Contains(bodies[3], "backup-signature") {
		t.Fatal("ongoing loop switched away from healthy model")
	}
}

func TestGeminiConcurrentFallback(t *testing.T) {
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "gemini-primary:") {
			w.WriteHeader(429)
			return
		}
		_, _ = io.WriteString(w, geminiTestOK)
	})
	g := &Gemini{Key: "k", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			msg, err := g.Complete(context.Background(), "sys", nil, nil)
			if err != nil || msg.Model != "gemini-backup" {
				t.Errorf("concurrent fallback failed: %+v %v", msg, err)
			}
		})
	}
	wg.Wait()
}

func TestGeminiFallbackConfigurationAndRetryHints(t *testing.T) {
	models, err := ParseGeminiFallbackModels("gemini-primary", " gemini-backup,gemini-primary,gemini-backup,,gemini-last ")
	if err != nil || strings.Join(models, ",") != "gemini-backup,gemini-last" {
		t.Fatalf("unexpected config %v %v", models, err)
	}
	if _, err := ParseGeminiFallbackModels("gemini-primary", "../../secret"); err == nil {
		t.Fatal("accepted invalid model")
	}
	if models, err := ParseGeminiFallbackModels("gemini-primary", ""); err != nil || len(models) != 0 {
		t.Fatal("cannot disable fallback")
	}
	raw := []byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"180s"}]}}`)
	if delay := geminiRetryDelay("120", raw); delay != 180*time.Second {
		t.Fatalf("ignored retry hint: %v", delay)
	}
	if delay := geminiRetryDelay("garbage", nil); delay != time.Minute {
		t.Fatalf("bad default cooldown: %v", delay)
	}
}

func TestGeminiDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	attempts := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Redirect(w, r, "https://other.invalid/collect", http.StatusTemporaryRedirect)
	})
	g := &Gemini{Key: "secret", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
	if _, err := g.Complete(context.Background(), "sys", nil, nil); err == nil || attempts != 1 {
		t.Fatalf("redirect was followed or retried: attempts=%d, error=%v", attempts, err)
	}
}

func TestGeminiCancellationDuringFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		cancel()
		w.WriteHeader(429)
	})
	g := &Gemini{Key: "k", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
	if _, err := g.Complete(ctx, "sys", nil, nil); !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled request continued: attempts=%d, error=%v", attempts, err)
	}
}

func TestGeminiFallbackOnRequestTimeout(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: geminiTestTransport(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, &net.DNSError{IsTimeout: true}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(geminiTestOK))}, nil
	})}
	g := &Gemini{Key: "k", Model: "gemini-primary", FallbackModels: []string{"gemini-backup"}, HTTP: client}
	msg, err := g.Complete(context.Background(), "sys", nil, nil)
	if err != nil || msg.Model != "gemini-backup" || attempts != 2 {
		t.Fatalf("timeout fallback failed: %+v %v, attempts %d", msg, err, attempts)
	}
}
