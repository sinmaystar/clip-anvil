package craftsman

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/compose"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

type Loader interface {
	Load(ctx context.Context, input GraphInput) (Context, error)
}

type GraphConfig struct {
	Loader             Loader
	ToolResponder      ToolCallingResponder
	NativeToolRegistry *agenttools.NativeRegistry
	CheckPointStore    compose.CheckPointStore
	CompileCallbacks   []compose.GraphCompileCallback
}

type Graph struct {
	runnable compose.Runnable[GraphInput, GraphOutput]
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.ToolResponder == nil || config.NativeToolRegistry == nil {
		return nil, ErrInvalidConfig
	}
	return newNativeToolLoopGraph(config)
}

func (g *Graph) Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
	if g == nil || g.runnable == nil {
		return GraphOutput{}, ErrInvalidConfig
	}
	runOptions := agenteino.RunOptions{}
	if len(options) > 0 {
		runOptions = options[0]
	}
	ctx, callOptions := agenteino.ApplyRunOptions(ctx, runOptions)
	return g.runnable.Invoke(ctx, input, callOptions...)
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
