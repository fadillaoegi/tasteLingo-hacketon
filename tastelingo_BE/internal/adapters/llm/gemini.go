package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"tastelingo/internal/application"
	"time"
)

// Gemini implements the LLM port with GenerateContent function calling. Model
// turns are echoed back verbatim (Raw) so thought signatures are preserved.
type Gemini struct {
	Key, Model     string
	BaseURL        string
	HTTP           *http.Client
	FallbackModels []string
	AutoDiscover   bool

	mu           sync.Mutex
	blocked      map[string]modelBlock
	now          func() time.Time // tests may supply a clock before using the adapter
	catalogMu    sync.Mutex
	catalog      []string
	catalogUntil time.Time
	catalogErr   error
}

type modelBlock struct {
	until time.Time
	err   error
}

const DefaultGeminiModel = "gemini-3.5-flash-lite"
const DefaultGeminiFallbackModels = "gemini-3.1-flash-lite,gemini-3.8-flash,gemini-3.6-flash"

// ParseGeminiFallbackModels validates and deduplicates the configured order.
func ParseGeminiFallbackModels(primary, csv string) ([]string, error) {
	if !ValidGeminiModel(primary) {
		return nil, errors.New("gemini: invalid primary model")
	}
	seen := map[string]bool{primary: true}
	var models []string
	for _, model := range strings.Split(csv, ",") {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if !ValidGeminiModel(model) {
			return nil, errors.New("gemini: invalid fallback model")
		}
		if !seen[model] {
			models = append(models, model)
			seen[model] = true
		}
	}
	return models, nil
}

// localID marks call ids the adapter invented; they are not sent back to Gemini.
const localID = "local-"

var geminiModel = regexp.MustCompile(`^gemini-[a-zA-Z0-9][a-zA-Z0-9._-]{0,119}$`)

func ValidGeminiModel(model string) bool { return geminiModel.MatchString(model) }
func (g *Gemini) Mode() string           { return "gemini" }

type part struct {
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}
type functionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}
type functionResponse struct {
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}
type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

func (g *Gemini) Complete(ctx context.Context, system string, history []application.Message, tools []application.ToolSpec) (application.Message, error) {
	if g.Key == "" || !ValidGeminiModel(g.Model) || g.HTTP == nil {
		return application.Message{}, errors.New("gemini: configure key, model and HTTP client")
	}
	models, err := ParseGeminiFallbackModels(g.Model, strings.Join(g.FallbackModels, ","))
	if err != nil {
		return application.Message{}, err
	}
	models = append([]string{g.Model}, models...)
	// Keep an ongoing tool loop on its last model when possible.
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Model == "" {
			continue
		}
		found := false
		for j, model := range models {
			if model == history[i].Model {
				found = true
				copy(models[1:j+1], models[:j])
				models[0] = model
				break
			}
		}
		if !found && g.AutoDiscover && textGeminiModel.MatchString(history[i].Model) && ValidGeminiModel(history[i].Model) {
			models = append([]string{history[i].Model}, models...)
		}
		break
	}
	var failures []error
	discovered := false
	for i := 0; ; i++ {
		if i == len(models) {
			if !g.AutoDiscover || discovered {
				break
			}
			discovered = true
			catalog, err := g.discoverModels(ctx)
			if err != nil {
				failures = append(failures, err)
				break
			}
			seen := map[string]bool{}
			for _, model := range models {
				seen[model] = true
			}
			for _, model := range catalog {
				if !seen[model] {
					models = append(models, model)
					seen[model] = true
				}
			}
			if i == len(models) {
				break
			}
		}
		model := models[i]
		if err := ctx.Err(); err != nil {
			return application.Message{}, err
		}
		now := time.Now()
		if g.now != nil {
			now = g.now()
		}
		g.mu.Lock()
		block := g.blocked[model]
		g.mu.Unlock()
		if now.Before(block.until) {
			failures = append(failures, block.err)
			continue
		}
		msg, retry, delay, err := g.completeModel(ctx, model, system, history, tools)
		if err == nil {
			msg.Model = model
			return msg, nil
		}
		if ctx.Err() != nil {
			return application.Message{}, ctx.Err()
		}
		if !retry {
			return application.Message{}, err
		}
		failures = append(failures, err)
		if g.now == nil {
			now = time.Now()
		} else {
			now = g.now()
		}
		g.mu.Lock()
		if g.blocked == nil {
			g.blocked = make(map[string]modelBlock)
		}
		if until := now.Add(delay); until.After(g.blocked[model].until) {
			g.blocked[model] = modelBlock{until: until, err: err}
		}
		g.mu.Unlock()
		slog.Warn("Gemini model unavailable; trying fallback", "model", model, "cooldown", delay, "error", err)
	}
	if err := ctx.Err(); err != nil {
		return application.Message{}, err
	}
	return application.Message{}, fmt.Errorf("gemini: all configured models unavailable or cooling down: %w", errors.Join(failures...))
}

func (g *Gemini) completeModel(ctx context.Context, model, system string, history []application.Message, tools []application.ToolSpec) (application.Message, bool, time.Duration, error) {
	contents := make([]json.RawMessage, 0, len(history))
	for _, m := range history {
		// Foreign signatures cannot be reused after a model switch. Rebuild only
		// those turns using Google's documented history-transfer placeholder.
		foreign := m.Role == application.RoleModel && m.Model != "" && m.Model != model
		if foreign {
			m.Raw = nil
		}
		raw, err := toContent(m)
		if err != nil {
			return application.Message{}, false, 0, err
		}
		if foreign && len(m.Calls) > 0 {
			var c content
			if err := json.Unmarshal(raw, &c); err != nil {
				return application.Message{}, false, 0, errors.New("gemini: invalid history")
			}
			for i := range c.Parts {
				if c.Parts[i].FunctionCall != nil {
					c.Parts[i].ThoughtSignature = "skip_thought_signature_validator"
					if strings.HasPrefix(c.Parts[i].FunctionCall.ID, localID) {
						c.Parts[i].FunctionCall.ID = ""
					}
				}
			}
			raw, err = json.Marshal(c)
			if err != nil {
				return application.Message{}, false, 0, err
			}
		}
		contents = append(contents, raw)
	}
	body := map[string]any{
		"systemInstruction": map[string]any{"parts": []part{{Text: system}}},
		"contents":          contents,
		"generationConfig":  map[string]any{"candidateCount": 1, "maxOutputTokens": 8192},
	}
	decls := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		decls = append(decls, map[string]any{"name": t.Name, "description": t.Description, "parametersJsonSchema": t.Parameters})
	}
	if len(decls) > 0 {
		body["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return application.Message{}, false, 0, errors.New("gemini: request could not be encoded")
	}
	base := g.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1beta/models/"+model+":generateContent", bytes.NewReader(payload))
	if err != nil {
		return application.Message{}, false, 0, errors.New("gemini: invalid request URL")
	}
	req.Header.Set("x-goog-api-key", g.Key)
	req.Header.Set("Content-Type", "application/json")
	client := *g.HTTP
	// Never forward the credential to a redirected endpoint.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		var networkErr net.Error
		if errors.As(err, &networkErr) && networkErr.Timeout() {
			return application.Message{}, true, time.Minute, fmt.Errorf("gemini: model %s request timed out", model)
		}
		return application.Message{}, false, 0, errors.New("gemini: request failed")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return application.Message{}, false, 0, errors.New("gemini: response unreadable")
	}
	if res.StatusCode != http.StatusOK {
		retry := res.StatusCode == 429 || res.StatusCode == 404 || res.StatusCode == 500 || res.StatusCode == 502 || res.StatusCode == 503 || res.StatusCode == 504
		return application.Message{}, retry, geminiRetryDelay(res.Header.Get("Retry-After"), raw), fmt.Errorf("gemini: model %s HTTP %d %s", model, res.StatusCode, providerStatus(raw))
	}
	var out struct {
		Candidates []struct {
			Content      json.RawMessage `json:"content"`
			FinishReason string          `json:"finishReason"`
		} `json:"candidates"`
	}
	if json.Unmarshal(raw, &out) != nil || len(out.Candidates) == 0 || len(out.Candidates[0].Content) == 0 {
		return application.Message{}, false, 0, errors.New("gemini: no candidate returned")
	}
	msg, err := fromContent(out.Candidates[0].Content)
	return msg, false, 0, err
}

// Honor server retry hints without sleeping in a learner's request.
func geminiRetryDelay(header string, raw []byte) time.Duration {
	delay := time.Minute
	if seconds, err := strconv.ParseInt(header, 10, 32); err == nil && seconds > 0 {
		delay = max(delay, time.Duration(seconds)*time.Second)
	} else if date, err := http.ParseTime(header); err == nil {
		delay = max(delay, time.Until(date))
	}
	var e struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil {
		for _, d := range e.Error.Details {
			if d.Type == "type.googleapis.com/google.rpc.RetryInfo" {
				if parsed, err := time.ParseDuration(d.RetryDelay); err == nil {
					delay = max(delay, parsed)
				}
			}
		}
	}
	return delay
}

func toContent(m application.Message) (json.RawMessage, error) {
	switch m.Role {
	case application.RoleModel:
		if len(m.Raw) > 0 {
			return m.Raw, nil
		}
		c := content{Role: "model"}
		if m.Text != "" {
			c.Parts = append(c.Parts, part{Text: m.Text})
		}
		for _, call := range m.Calls {
			c.Parts = append(c.Parts, part{FunctionCall: &functionCall{ID: call.ID, Name: call.Name, Args: call.Args}})
		}
		return json.Marshal(c)
	case application.RoleTool:
		c := content{Role: "user"}
		for _, r := range m.Results {
			id := r.CallID
			if strings.HasPrefix(id, localID) {
				id = ""
			}
			c.Parts = append(c.Parts, part{FunctionResponse: &functionResponse{ID: id, Name: r.Name, Response: r.Content}})
		}
		return json.Marshal(c)
	default:
		return json.Marshal(content{Role: "user", Parts: []part{{Text: m.Text}}})
	}
}

func fromContent(raw json.RawMessage) (application.Message, error) {
	var c content
	if err := json.Unmarshal(raw, &c); err != nil {
		return application.Message{}, errors.New("gemini: unexpected content")
	}
	msg := application.Message{Role: application.RoleModel, Raw: raw}
	for i, p := range c.Parts {
		switch {
		case p.FunctionCall != nil:
			id := p.FunctionCall.ID
			if id == "" {
				id = fmt.Sprintf("%s%d", localID, i)
			}
			args := p.FunctionCall.Args
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			msg.Calls = append(msg.Calls, application.ToolCall{ID: id, Name: p.FunctionCall.Name, Args: args})
		case p.Text != "" && !p.Thought:
			if msg.Text != "" {
				msg.Text += "\n"
			}
			msg.Text += p.Text
		}
	}
	return msg, nil
}

// providerStatus extracts only the status code name; provider messages may echo request data.
func providerStatus(raw []byte) string {
	var e struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	return e.Error.Status
}
