package producer

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

const producerLoopStateArgumentKey = "_clipanvil_producer_loop_state_key"

type producerLoopToolStateStore struct {
	byKey    sync.Map
	byCallID sync.Map
}

type storedProducerLoopState struct {
	key   string
	state ProducerLoopState
}

func newProducerLoopToolStateStore() *producerLoopToolStateStore {
	return &producerLoopToolStateStore{}
}

func (s *producerLoopToolStateStore) rememberKey(key string, state ProducerLoopState) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.byKey.Store(key, state)
}

func (s *producerLoopToolStateStore) rememberCall(toolCallID string, state ProducerLoopState) {
	s.rememberCallWithKey(toolCallID, "", state)
}

func (s *producerLoopToolStateStore) rememberCallWithKey(toolCallID string, key string, state ProducerLoopState) {
	if s == nil || strings.TrimSpace(toolCallID) == "" {
		return
	}
	s.byCallID.Store(toolCallID, storedProducerLoopState{key: key, state: state})
}

func (s *producerLoopToolStateStore) stateByKey(key string) (ProducerLoopState, bool) {
	if s == nil || strings.TrimSpace(key) == "" {
		return ProducerLoopState{}, false
	}
	value, ok := s.byKey.Load(key)
	if !ok {
		return ProducerLoopState{}, false
	}
	state, ok := value.(ProducerLoopState)
	return state, ok
}

func (s *producerLoopToolStateStore) stateByCall(toolCallID string) (ProducerLoopState, bool) {
	if s == nil || strings.TrimSpace(toolCallID) == "" {
		return ProducerLoopState{}, false
	}
	value, ok := s.byCallID.Load(toolCallID)
	if !ok {
		return ProducerLoopState{}, false
	}
	stored, ok := value.(storedProducerLoopState)
	return stored.state, ok
}

func (s *producerLoopToolStateStore) forgetCall(toolCallID string) {
	if s == nil || strings.TrimSpace(toolCallID) == "" {
		return
	}
	value, ok := s.byCallID.LoadAndDelete(toolCallID)
	if !ok {
		return
	}
	if stored, ok := value.(storedProducerLoopState); ok && strings.TrimSpace(stored.key) != "" {
		s.byKey.Delete(stored.key)
	}
}

func producerToolNodeForConfig(ctx context.Context, config GraphConfig, stateStore *producerLoopToolStateStore) (*compose.ToolsNode, []*schema.ToolInfo, error) {
	if config.NativeToolRegistry == nil {
		return nil, nil, NewAgentError("agent_native_tool_registry_missing", "producer native tool registry is not configured")
	}
	infos, err := config.NativeToolRegistry.ToolInfos(ctx)
	if err != nil {
		return nil, nil, err
	}
	node, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools:               config.NativeToolRegistry.BaseTools(),
		ExecuteSequentially: true,
		ToolCallMiddlewares: []compose.ToolMiddleware{nativeProducerToolRuntimeMiddleware(stateStore)},
	})
	if err != nil {
		return nil, nil, err
	}
	return node, infos, nil
}

func nativeProducerToolRuntimeMiddleware(stateStore *producerLoopToolStateStore) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if input == nil {
					return next(ctx, input)
				}
				args := map[string]any{}
				if input.Arguments != "" {
					_ = json.Unmarshal([]byte(input.Arguments), &args)
				}
				if stateKey, _ := args[producerLoopStateArgumentKey].(string); stateKey != "" && stateStore != nil {
					delete(args, producerLoopStateArgumentKey)
					if raw, err := json.Marshal(args); err == nil {
						input.Arguments = string(raw)
					}
					if state, ok := stateStore.stateByKey(stateKey); ok {
						stateStore.rememberCallWithKey(input.CallID, stateKey, state)
						ctx = agenttools.WithNativeRuntimeContext(ctx, agenttools.NativeRuntimeContext{
							WorkspaceID: state.Context.Input.WorkspaceID,
							ThreadID:    state.Context.Input.ThreadID,
							TaskID:      state.Context.Input.TaskID,
							ToolCallID:  input.CallID,
						})
					}
				}
				runtime, _ := agenttools.NativeRuntimeFromContext(ctx)
				if strings.TrimSpace(runtime.ToolCallID) == "" {
					runtime.ToolCallID = input.CallID
				}
				if sink, ok := agenttools.NativeToolTraceSinkFromContext(ctx); ok && sink != nil {
					if err := sink.NativeToolCallStarted(ctx, runtime, agenttools.NativeToolTrace{
						ToolName:  input.Name,
						Arguments: args,
					}); err != nil {
						return nil, err
					}
				}
				out, err := next(ctx, input)
				trace := agenttools.NativeToolTrace{ToolName: input.Name}
				if out != nil {
					trace.Result = out.Result
				}
				if err != nil {
					trace.Error = err.Error()
				}
				if _, interrupted := compose.IsInterruptRerunError(err); interrupted {
					return out, err
				}
				if _, interrupted := compose.ExtractInterruptInfo(err); interrupted {
					return out, err
				}
				if sink, ok := agenttools.NativeToolTraceSinkFromContext(ctx); ok && sink != nil {
					if traceErr := sink.NativeToolCallCompleted(ctx, runtime, trace); traceErr != nil {
						if err != nil {
							return out, err
						}
						return out, traceErr
					}
				}
				return out, err
			}
		},
	}
}
