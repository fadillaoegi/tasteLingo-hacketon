package qloo

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"tastelingo/internal/application"
	"tastelingo/internal/domain"
)

// Mock returns synthetic, clearly labelled entities so the app runs before the
// Qloo key arrives. Nothing here is Qloo data, and every name ends in "(mock)".
type Mock struct{}

func (Mock) Mode() string { return domain.SourceMock }

func (Mock) Search(_ context.Context, query string, t domain.EntityType, take int) ([]domain.Entity, error) {
	types := domain.MediaTypes
	if t != "" {
		types = []domain.EntityType{t}
	}
	query = strings.TrimSpace(query)
	var out []domain.Entity
	for _, typ := range types {
		if len(out) >= take {
			break
		}
		out = append(out, mockEntity("search:"+strings.ToLower(query), typ, 0, query, 0))
	}
	return out, nil
}

var mockCatalog = map[domain.EntityType][]struct {
	name string
	tags []string
}{
	domain.Movie:   {{"The Lantern Bathhouse", []string{"animation", "coming of age", "fantasy"}}, {"Paper Kites Over Harbor", []string{"drama", "friendship"}}, {"Night Train to Willow", []string{"adventure", "fantasy"}}, {"The Quiet Interview", []string{"workplace", "drama"}}},
	domain.TVShow:  {{"Hallway Seven", []string{"workplace comedy", "ensemble"}}, {"Small Town Signals", []string{"mystery", "slice of life"}}, {"The Bakery Shift", []string{"comedy", "food"}}},
	domain.Book:    {{"Letters from the Hill House", []string{"young adult", "fantasy"}}, {"First Job Field Notes", []string{"career", "self-improvement"}}, {"The Cartographer's Daughter", []string{"adventure"}}},
	domain.Podcast: {{"Five-Minute Mindset", []string{"self-improvement", "short episodes"}}, {"Career Stories Weekly", []string{"careers", "interviews"}}, {"Slow English Mornings", []string{"learning", "daily life"}}},
	domain.Artist:  {{"The Rooftop Choir", []string{"indie", "folk"}}, {"Mira & the Tides", []string{"indie pop"}}, {"Static Lanterns", []string{"alternative", "indie"}}},
}

func (Mock) Insights(_ context.Context, in application.InsightsQuery) ([]domain.Entity, error) {
	items := mockCatalog[in.Type]
	out := make([]domain.Entity, 0, len(items))
	for i, it := range items {
		if i >= in.Take {
			break
		}
		e := mockEntity("insight:"+it.name, in.Type, 2001+i*3, it.name, 0.92-float64(i)*0.07)
		e.Tags = it.tags
		out = append(out, e)
	}
	return out, nil
}

func (Mock) Tags(_ context.Context, query string, take int) ([]domain.Tag, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	out := []domain.Tag{}
	for _, name := range []string{q, q + " drama", q + " comedy"} {
		if len(out) >= take {
			break
		}
		out = append(out, domain.Tag{ID: "mock-tag-" + slug(name), Name: name + " (mock)", Type: "urn:tag:genre:mock"})
	}
	return out, nil
}

func mockEntity(seed string, t domain.EntityType, year int, name string, affinity float64) domain.Entity {
	sum := sha1.Sum([]byte(seed + string(t)))
	return domain.Entity{
		ID:          "mock-" + hex.EncodeToString(sum[:8]),
		Name:        fmt.Sprintf("%s (mock)", name),
		Type:        t,
		Year:        year,
		Description: fmt.Sprintf("Synthetic %s used while the Qloo API key is pending. Not real Qloo data.", strings.ReplaceAll(string(t), "_", " ")),
		Affinity:    affinity,
		Source:      domain.SourceMock,
	}
}

func slug(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, s)
}
