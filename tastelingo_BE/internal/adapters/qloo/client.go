package qloo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
)

// Client calls the Qloo hackathon API. Response shapes follow the hackathon
// developer guide and are parsed leniently until verified against live data.
type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
}

func (c *Client) Mode() string { return domain.SourceQloo }

const urnPrefix = "urn:entity:"

func (c *Client) Search(ctx context.Context, query string, t domain.EntityType, take int) ([]domain.Entity, error) {
	q := url.Values{"query": {query}, "take": {strconv.Itoa(take)}}
	if t != "" {
		q.Set("types", urnPrefix+string(t))
	}
	var body struct {
		Results []entityJSON `json:"results"`
	}
	if err := c.get(ctx, "/search", q, &body); err != nil {
		return nil, err
	}
	return toEntities(body.Results, t), nil
}

func (c *Client) Insights(ctx context.Context, in application.InsightsQuery) ([]domain.Entity, error) {
	q := url.Values{"filter.type": {urnPrefix + string(in.Type)}, "take": {strconv.Itoa(in.Take)}}
	if len(in.EntityIDs) > 0 {
		q.Set("signal.interests.entities", strings.Join(in.EntityIDs, ","))
	}
	if len(in.TagIDs) > 0 {
		q.Set("signal.interests.tags", strings.Join(in.TagIDs, ","))
	}
	var body struct {
		Results struct {
			Entities []entityJSON `json:"entities"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/v2/insights", q, &body); err != nil {
		return nil, err
	}
	return toEntities(body.Results.Entities, in.Type), nil
}

func (c *Client) Tags(ctx context.Context, query string, take int) ([]domain.Tag, error) {
	q := url.Values{"filter.query": {query}, "take": {strconv.Itoa(take)}}
	var body struct {
		Results struct {
			Tags []struct {
				ID    string `json:"id"`
				TagID string `json:"tag_id"`
				Name  string `json:"name"`
				Type  string `json:"type"`
			} `json:"tags"`
		} `json:"results"`
	}
	if err := c.get(ctx, "/v2/tags", q, &body); err != nil {
		return nil, err
	}
	tags := make([]domain.Tag, 0, len(body.Results.Tags))
	for _, t := range body.Results.Tags {
		id := t.ID
		if id == "" {
			id = t.TagID
		}
		tags = append(tags, domain.Tag{ID: id, Name: t.Name, Type: t.Type})
	}
	return tags, nil
}

type entityJSON struct {
	EntityID   string   `json:"entity_id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Types      []string `json:"types"`
	Properties struct {
		Description      string          `json:"description"`
		ShortDescription string          `json:"short_description"`
		ReleaseYear      json.RawMessage `json:"release_year"`
		Image            struct {
			URL string `json:"url"`
		} `json:"image"`
	} `json:"properties"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	Query struct {
		Affinity float64 `json:"affinity"`
	} `json:"query"`
}

func toEntities(list []entityJSON, fallback domain.EntityType) []domain.Entity {
	out := make([]domain.Entity, 0, len(list))
	for _, r := range list {
		if r.EntityID == "" || r.Name == "" {
			continue
		}
		e := domain.Entity{ID: r.EntityID, Name: r.Name, Type: entityType(r, fallback), Description: r.Properties.Description, ImageURL: r.Properties.Image.URL, Affinity: r.Query.Affinity, Source: domain.SourceQloo}
		if e.Description == "" {
			e.Description = r.Properties.ShortDescription
		}
		e.Year = year(r.Properties.ReleaseYear)
		for _, t := range r.Tags {
			if t.Name != "" {
				e.Tags = append(e.Tags, t.Name)
			}
		}
		out = append(out, e)
	}
	return out
}

func entityType(r entityJSON, fallback domain.EntityType) domain.EntityType {
	for _, urn := range append([]string{r.Type}, r.Types...) {
		if t := domain.EntityType(strings.TrimPrefix(urn, urnPrefix)); domain.ValidEntityType(t) {
			return t
		}
	}
	return fallback
}

// release_year has been seen as both number and string in API examples.
func year(raw json.RawMessage) int {
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(s))
	}
	return n
}

var ErrUnavailable = errors.New("taste graph unavailable")

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.Key)
	req.Header.Set("Accept", "application/json")
	client := *c.HTTP
	// Never forward the key to a redirected host.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s request failed", ErrUnavailable, path)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("%w: %s response unreadable", ErrUnavailable, path)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s returned HTTP %d", ErrUnavailable, path, res.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: %s returned unexpected JSON", ErrUnavailable, path)
	}
	return nil
}
