package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"tastelingo/internal/domain"
	"time"
)

// Tool is one capability the agent may call. A Terminal tool ends the run once
// it succeeds, so the final structured output always passes its validation.
type Tool struct {
	Spec     ToolSpec
	Terminal bool
	Run      func(context.Context, json.RawMessage) (any, error)
	// Redact, when set, rewrites the arguments before they enter the visible trace.
	Redact func(json.RawMessage) json.RawMessage
}

type Agent struct {
	LLM      LLM
	MaxTurns int
	Now      func() time.Time
}

var ErrAgentIncomplete = errors.New("agent stopped before finishing its task")

// Run drives the tool-calling loop and appends every thought, call and result
// to run.Steps so the trace can be shown to the learner and the judges.
func (a Agent) Run(ctx context.Context, run *domain.AgentRun, system, task string, tools []Tool) error {
	byName := make(map[string]Tool, len(tools))
	specs := make([]ToolSpec, 0, len(tools))
	for _, t := range tools {
		byName[t.Spec.Name] = t
		specs = append(specs, t.Spec)
	}
	history := []Message{{Role: RoleUser, Text: task}}
	for turn := 0; turn < a.MaxTurns; turn++ {
		reply, err := a.LLM.Complete(ctx, system, history, specs)
		if err != nil {
			a.record(run, domain.TraceStep{Kind: domain.StepError, Output: err.Error()})
			return err
		}
		history = append(history, reply)
		if reply.Text != "" {
			kind := domain.StepThought
			if len(reply.Calls) == 0 {
				kind = domain.StepFinal
			}
			a.record(run, domain.TraceStep{Kind: kind, Model: reply.Model, Output: reply.Text})
		}
		if len(reply.Calls) == 0 {
			return ErrAgentIncomplete
		}
		results := make([]ToolResult, 0, len(reply.Calls))
		done := false
		for _, call := range reply.Calls {
			input := call.Args
			if t, ok := byName[call.Name]; ok && t.Redact != nil {
				input = t.Redact(input)
			}
			a.record(run, domain.TraceStep{Kind: domain.StepToolCall, Tool: call.Name, Model: reply.Model, Input: input})
			started := a.Now()
			content, ok := a.call(ctx, byName, call)
			a.record(run, domain.TraceStep{Kind: domain.StepToolResult, Tool: call.Name, Output: summarize(content), DurationMS: a.Now().Sub(started).Milliseconds()})
			results = append(results, ToolResult{CallID: call.ID, Name: call.Name, Content: content})
			done = done || ok && byName[call.Name].Terminal
		}
		if done {
			return nil
		}
		history = append(history, Message{Role: RoleTool, Results: results})
	}
	a.record(run, domain.TraceStep{Kind: domain.StepError, Output: fmt.Sprintf("turn limit %d reached", a.MaxTurns)})
	return ErrAgentIncomplete
}

// call returns the JSON fed back to the model; tool errors are reported to the
// model as data so it can correct its arguments.
func (a Agent) call(ctx context.Context, tools map[string]Tool, call ToolCall) (json.RawMessage, bool) {
	t, ok := tools[call.Name]
	if !ok {
		return errorJSON("unknown tool " + call.Name), false
	}
	out, err := t.Run(ctx, call.Args)
	if err != nil {
		return errorJSON(err.Error()), false
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return errorJSON("tool output could not be encoded"), false
	}
	return raw, true
}

func (a Agent) record(run *domain.AgentRun, step domain.TraceStep) {
	step.Index = len(run.Steps)
	step.At = a.Now().UTC()
	run.Steps = append(run.Steps, step)
}

func errorJSON(msg string) json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"error": msg})
	return raw
}

func summarize(raw json.RawMessage) string {
	const limit = 1500
	if len(raw) > limit {
		return string(raw[:limit]) + "…"
	}
	return string(raw)
}

// Ask makes one tool-free model call and records it as the run's final step.
func (a Agent) Ask(ctx context.Context, run *domain.AgentRun, system, task string) (string, error) {
	reply, err := a.LLM.Complete(ctx, system, []Message{{Role: RoleUser, Text: task}}, nil)
	if err != nil {
		a.record(run, domain.TraceStep{Kind: domain.StepError, Output: err.Error()})
		return "", err
	}
	a.record(run, domain.TraceStep{Kind: domain.StepFinal, Model: reply.Model, Output: reply.Text})
	return reply.Text, nil
}
