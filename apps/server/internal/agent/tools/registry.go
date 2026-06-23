package tools

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrDuplicateTool = errors.New("duplicate agent tool")

type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Result      map[string]any
	Safety      SafetySpec
	Timeout     time.Duration
	Visibility  VisibilitySpec
}

type SafetySpec struct {
	ReadOnly              bool
	RequiresHITL          bool
	WritesCanvas          bool
	UsesProductionService bool
	MaxCallsPerTurn       int
}

type VisibilitySpec struct {
	ShowCallMessage   bool
	ShowResultMessage bool
	UserLabel         string
}

type ExecuteInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Arguments   map[string]any
}

type ExecuteOutput struct {
	Result  map[string]any
	Summary string
}

type Executor interface {
	Definition() Definition
	Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error)
}

type Registry struct {
	tools map[string]Executor
}

func NewRegistry(tools ...Executor) (*Registry, error) {
	registry := &Registry{tools: map[string]Executor{}}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := tool.Definition().Name
		if _, exists := registry.tools[name]; exists {
			return nil, ErrDuplicateTool
		}
		registry.tools[name] = tool
	}
	return registry, nil
}

func (r *Registry) Definition(name string) (Definition, bool) {
	tool, ok := r.tools[name]
	if !ok {
		return Definition{}, false
	}
	return tool.Definition(), true
}

func (r *Registry) Executor(name string) (Executor, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Executors() []Executor {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Executor, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}
