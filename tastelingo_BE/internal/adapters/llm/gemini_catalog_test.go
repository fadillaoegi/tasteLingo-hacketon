package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"tastelingo/internal/application"
	"testing"
)

func TestGeminiDiscoversCompatibleBackupsAfterConfiguredModelsAreLimited(t *testing.T) {
	var calls []string
	catalogCalls := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Header.Get("x-goog-api-key") != "secret" {
			t.Fatal("catalog credential missing")
		}
		if r.Method == "GET" {
			catalogCalls++
			if r.URL.Query().Get("pageToken") == "page2" {
				io.WriteString(w, `{"models":[{"name":"models/gemini-3.5-flash-lite","supportedGenerationMethods":["generateContent"]}]}`)
				return
			}
			io.WriteString(w, `{"nextPageToken":"page2","models":[
				{"name":"models/gemini-3.1-pro-preview","supportedGenerationMethods":["generateContent"]},
				{"name":"models/gemini-3.5-flash-tts","supportedGenerationMethods":["generateContent"]},
				{"name":"models/gemini-3.1-flash-image","supportedGenerationMethods":["generateContent"]},
				{"name":"models/gemini-3.6-flash","supportedGenerationMethods":["embedContent"]}]}`)
			return
		}
		if strings.Contains(r.URL.Path, "gemini-primary:") || strings.Contains(r.URL.Path, "gemini-configured:") {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"status":"RESOURCE_EXHAUSTED"}}`)
			return
		}
		if !strings.Contains(r.URL.Path, "gemini-3.5-flash-lite:") {
			t.Fatalf("wrong discovered model: %s", r.URL.Path)
		}
		io.WriteString(w, geminiTestOK)
	})
	g := &Gemini{Key: "secret", Model: "gemini-primary", FallbackModels: []string{"gemini-configured"}, AutoDiscover: true, HTTP: client}
	for range 2 {
		msg, err := g.Complete(context.Background(), "sys", nil, nil)
		if err != nil || msg.Model != "gemini-3.5-flash-lite" {
			t.Fatalf("dynamic fallback failed: %+v %v", msg, err)
		}
	}
	if catalogCalls != 2 {
		t.Fatalf("expected two pages once, got %d; calls=%v", catalogCalls, calls)
	}
}

func TestGeminiHealthyOrUnauthorizedRequestsDoNotDiscoverModels(t *testing.T) {
	for _, status := range []int{200, 401} {
		client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				t.Fatal("unnecessary model discovery")
			}
			w.WriteHeader(status)
			if status == 200 {
				io.WriteString(w, geminiTestOK)
			}
		})
		g := &Gemini{Key: "k", Model: "gemini-primary", AutoDiscover: true, HTTP: client}
		_, err := g.Complete(context.Background(), "sys", nil, nil)
		if (err != nil) != (status != 200) {
			t.Fatalf("status %d: %v", status, err)
		}
	}
}

func TestGeminiCatalogFiltersSpecializedModels(t *testing.T) {
	for _, name := range []string{"gemini-3.8-flash", "gemini-3.5-flash-lite", "gemini-flash-latest", "gemini-2.5-pro", "gemini-3.1-pro-preview", "gemini-2.5-flash-preview-09-2025"} {
		if !textGeminiModel.MatchString(name) {
			t.Errorf("text model excluded: %s", name)
		}
	}
	for _, name := range []string{"gemini-3.8-flash-tts", "gemini-3.1-flash-image", "gemini-embedding-001", "gemini-3.8-live", "gemini-3.1-pro-preview-customtools", "veo-3", "../../private"} {
		if textGeminiModel.MatchString(name) {
			t.Errorf("specialized model accepted: %s", name)
		}
	}
}

func TestGeminiKeepsDiscoveredModelForOngoingConversation(t *testing.T) {
	primaryCalls := 0
	client := geminiTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{"models":[{"name":"models/gemini-3.5-flash-lite","supportedGenerationMethods":["generateContent"]}]}`)
			return
		}
		if strings.Contains(r.URL.Path, "gemini-primary:") {
			primaryCalls++
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, geminiTestOK)
	})
	g := &Gemini{Key: "k", Model: "gemini-primary", AutoDiscover: true, HTTP: client}
	first, err := g.Complete(context.Background(), "sys", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	delete(g.blocked, "gemini-primary")
	g.mu.Unlock()
	second, err := g.Complete(context.Background(), "sys", []application.Message{first}, nil)
	if err != nil || second.Model != first.Model || primaryCalls != 1 {
		t.Fatalf("lost discovered model: %+v %v, primary calls %d", second, err, primaryCalls)
	}
}
