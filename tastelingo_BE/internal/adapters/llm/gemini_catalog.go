package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Only general-purpose text models can continue this application's tool loops.
// Image, audio, live, embedding and specialized agent models are not substitutes.
var textGeminiModel = regexp.MustCompile(`^gemini-(?:[0-9]+(?:\.[0-9]+)?-)?(?:flash(?:-lite)?|pro)(?:-(?:preview|latest|exp)(?:-[0-9-]+)?)?$`)

// discoverModels is a last-resort catalog lookup, so healthy configured models
// never pay for an extra network round trip. Cache across requests and callers.
func (g *Gemini) discoverModels(ctx context.Context) ([]string, error) {
	g.catalogMu.Lock()
	defer g.catalogMu.Unlock()
	if time.Now().Before(g.catalogUntil) {
		return append([]string(nil), g.catalog...), g.catalogErr
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	models, err := g.fetchCatalog(ctx)
	if ctx.Err() != nil {
		return nil, err
	}
	g.catalog, g.catalogErr = models, err
	g.catalogUntil = time.Now().Add(15 * time.Minute)
	if err != nil {
		g.catalogUntil = time.Now().Add(time.Minute)
	}
	return append([]string(nil), models...), err
}

func (g *Gemini) fetchCatalog(ctx context.Context) ([]string, error) {
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	client := *g.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var models []string
	seen := map[string]bool{}
	page := ""
	for range 10 {
		q := url.Values{"pageSize": {"100"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1beta/models?"+q.Encode(), nil)
		if err != nil {
			return nil, errors.New("gemini: invalid catalog URL")
		}
		req.Header.Set("x-goog-api-key", g.Key)
		res, err := client.Do(req)
		if err != nil {
			return nil, errors.New("gemini: model discovery unavailable")
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, errors.New("gemini: model catalog unreadable")
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini: model discovery HTTP %d %s", res.StatusCode, providerStatus(raw))
		}
		var body struct {
			Models []struct {
				Name    string   `json:"name"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			Next string `json:"nextPageToken"`
		}
		if json.Unmarshal(raw, &body) != nil {
			return nil, errors.New("gemini: unexpected model catalog")
		}
		for _, m := range body.Models {
			name := strings.TrimPrefix(m.Name, "models/")
			if !textGeminiModel.MatchString(name) || !ValidGeminiModel(name) || seen[name] {
				continue
			}
			for _, method := range m.Methods {
				if method == "generateContent" {
					models = append(models, name)
					seen[name] = true
					break
				}
			}
		}
		if body.Next == "" {
			break
		}
		if body.Next == page {
			return nil, errors.New("gemini: repeated model catalog page")
		}
		page = body.Next
	}
	// Try light text models before larger reasoning models. Configured priority
	// is preserved separately; this only orders automatically discovered backups.
	priority := func(model string) int {
		if strings.Contains(model, "flash-lite") {
			return 0
		}
		if strings.Contains(model, "flash") {
			return 1
		}
		return 2
	}
	sort.SliceStable(models, func(i, j int) bool { return priority(models[i]) < priority(models[j]) })
	return models, nil
}
