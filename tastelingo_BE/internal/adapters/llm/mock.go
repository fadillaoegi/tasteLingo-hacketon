package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"tastelingo/internal/application"
)

// Mock is a scripted model used before a real LLM is configured. It follows the
// same tool protocol as a real model so every agent flow is exercised end to end.
// All of its text is prefixed with [mock] so it is never mistaken for model output.
type Mock struct{}

func (Mock) Mode() string { return "mock" }

var mockTypes = []string{"movie", "tv_show", "podcast", "book"}

type mockTask struct {
	Task    string `json:"task"`
	Learner struct {
		Level        string `json:"level"`
		Goal         string `json:"goal"`
		DailyMinutes int    `json:"daily_minutes"`
	} `json:"learner"`
	Feedback []struct {
		Day  int    `json:"day"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"feedback"`
	ExcludedIDs []string `json:"excluded_ids"`
	Item        struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	} `json:"item"`
}

func (m Mock) Complete(_ context.Context, _ string, history []application.Message, _ []application.ToolSpec) (application.Message, error) {
	if len(history) == 0 {
		return application.Message{}, errors.New("mock: empty conversation")
	}
	var task mockTask
	if err := json.Unmarshal([]byte(history[0].Text), &task); err != nil {
		return application.Message{}, errors.New("mock: task is not JSON")
	}
	switch task.Task {
	case "llm_only":
		return text(mockLLMOnly(task)), nil
	case "attempt_feedback":
		return text("[mock] Good effort: your answer is on topic. Try one longer sentence that uses a new word from today's list."), nil
	case "exercise":
		return mockExercise(task), nil
	case "plan", "adapt":
		return mockPlan(task, history), nil
	}
	return application.Message{}, fmt.Errorf("mock: unknown task %q", task.Task)
}

func text(s string) application.Message {
	return application.Message{Role: application.RoleModel, Text: s}
}

func mockPlan(task mockTask, history []application.Message) application.Message {
	last := history[len(history)-1]
	if last.Role == application.RoleUser {
		calls := make([]application.ToolCall, 0, len(mockTypes))
		for i, t := range mockTypes {
			args, _ := json.Marshal(map[string]any{"type": t, "take": 4})
			calls = append(calls, application.ToolCall{ID: fmt.Sprintf("mock-%d", i), Name: "get_taste_insights", Args: args})
		}
		return application.Message{Role: application.RoleModel, Text: "[mock] Collecting taste insights across movies, TV, podcasts and books.", Calls: calls}
	}
	excluded := map[string]bool{}
	for _, id := range task.ExcludedIDs {
		excluded[id] = true
	}
	byType := map[string][]map[string]any{}
	for _, r := range last.Results {
		var out struct {
			Entities []map[string]any `json:"entities"`
			Error    string           `json:"error"`
		}
		_ = json.Unmarshal(r.Content, &out)
		if out.Error != "" {
			return text("[mock] A tool failed: " + out.Error)
		}
		for _, e := range out.Entities {
			if id, _ := e["id"].(string); excluded[id] {
				continue
			}
			t, _ := e["type"].(string)
			byType[t] = append(byType[t], e)
		}
	}
	focus := []string{"listening", "vocabulary", "listening", "reading", "speaking", "listening", "writing"}
	var days []map[string]any
	for day := 1; len(days) < 7; {
		before := len(days)
		for _, t := range mockTypes {
			if len(days) == 7 || len(byType[t]) == 0 {
				continue
			}
			e := byType[t][0]
			byType[t] = byType[t][1:]
			days = append(days, map[string]any{
				"day": day, "entity_id": e["id"], "focus": focus[day-1], "minutes": task.Learner.DailyMinutes,
				"activity": fmt.Sprintf("Spend %d minutes with %v and note 8 new phrases.", task.Learner.DailyMinutes, e["name"]),
				"why":      "[mock] Picked from taste insights for the learner's favorites.",
			})
			day++
		}
		if len(days) == before {
			return text("[mock] Not enough insight results for 7 days.")
		}
	}
	args := map[string]any{
		"themes":  []string{"[mock] storytelling", "[mock] personal growth"},
		"summary": fmt.Sprintf("[mock] 7-day %s plan for a %s learner.", task.Learner.Goal, task.Learner.Level),
		"days":    days,
	}
	name := "build_plan"
	if task.Task == "adapt" {
		name = "adapt_plan"
		notes := make([]string, 0, len(task.Feedback))
		for _, f := range task.Feedback {
			notes = append(notes, fmt.Sprintf("day %d %s (%s)", f.Day, f.Name, f.Kind))
		}
		args["changes"] = "[mock] Replanned after feedback: " + strings.Join(notes, "; ") + ". Disliked items were removed."
	}
	raw, _ := json.Marshal(args)
	return application.Message{Role: application.RoleModel, Text: "[mock] Building the plan from the insight results.", Calls: []application.ToolCall{{ID: "mock-plan", Name: name, Args: raw}}}
}

// mockLLMOnly imitates a generic chatbot answer: popular picks unrelated to the learner's taste.
func mockLLMOnly(task mockTask) string {
	titles := []struct{ title, typ string }{
		{"A Famous Sitcom (mock)", "tv_show"}, {"A Box-Office Blockbuster (mock)", "movie"}, {"A Classic Novel (mock)", "book"},
		{"A Top-Charting Podcast (mock)", "podcast"}, {"A Popular Pop Singer (mock)", "artist"}, {"An Award-Winning Drama (mock)", "movie"},
		{"A Bestselling Self-Help Book (mock)", "book"},
	}
	days := make([]map[string]any, 0, 7)
	for i, t := range titles {
		days = append(days, map[string]any{"day": i + 1, "title": t.title, "type": t.typ, "activity": "Watch, read or listen and note new words.", "focus": "listening", "minutes": task.Learner.DailyMinutes, "why": "[mock] Popular with English learners."})
	}
	raw, _ := json.Marshal(map[string]any{"themes": []string{"[mock] popular content"}, "summary": "[mock] A general plan of well-known English content.", "days": days})
	return "```json\n" + string(raw) + "\n```"
}

func mockExercise(task mockTask) application.Message {
	topic := "stories"
	if len(task.Item.Tags) > 0 {
		topic = task.Item.Tags[0]
	}
	vocab := []map[string]string{}
	for _, w := range []string{"plot", "character", "setting", "theme", "recommend", "memorable"} {
		vocab = append(vocab, map[string]string{"term": w, "meaning": "[mock] meaning of " + w, "example": fmt.Sprintf("[mock] The %s of %s is memorable.", w, task.Item.Name)})
	}
	args, _ := json.Marshal(map[string]any{
		"vocabulary": vocab,
		"questions": []map[string]any{
			{"question": fmt.Sprintf("[mock] Which genre fits %s?", task.Item.Name), "options": []string{topic, "cooking show", "sports news", "weather report"}, "answer": 0},
			{"question": "[mock] Which word describes the main person in a story?", "options": []string{"setting", "character", "theme", "plot"}, "answer": 1},
			{"question": "[mock] Which word means 'to suggest something good'?", "options": []string{"recommend", "remember", "repeat", "return"}, "answer": 0},
		},
		"prompt": map[string]string{"kind": "speaking", "text": fmt.Sprintf("[mock] In a job interview, describe in 3 sentences what %s taught you.", task.Item.Name)},
	})
	return application.Message{Role: application.RoleModel, Text: "[mock] Writing the exercise from the item's metadata.", Calls: []application.ToolCall{{ID: "mock-exercise", Name: "generate_exercise", Args: args}}}
}
