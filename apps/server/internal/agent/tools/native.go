package tools

import (
	"context"
	"errors"
	"sort"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrDuplicateNativeTool = errors.New("duplicate native agent tool")

type NativeTool interface {
	einotool.InvokableTool
}

type NativeRuntimeContext struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	ToolCallID  string
}

type nativeRuntimeContextKey struct{}

func WithNativeRuntimeContext(ctx context.Context, runtime NativeRuntimeContext) context.Context {
	return context.WithValue(ctx, nativeRuntimeContextKey{}, runtime)
}

func NativeRuntimeFromContext(ctx context.Context) (NativeRuntimeContext, bool) {
	runtime, ok := ctx.Value(nativeRuntimeContextKey{}).(NativeRuntimeContext)
	return runtime, ok
}

type NativeRegistry struct {
	tools map[string]NativeTool
}

func NewNativeRegistry(tools ...NativeTool) (*NativeRegistry, error) {
	registry := &NativeRegistry{tools: map[string]NativeTool{}}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		info, err := tool.Info(context.Background())
		if err != nil {
			return nil, err
		}
		if _, exists := registry.tools[info.Name]; exists {
			return nil, ErrDuplicateNativeTool
		}
		registry.tools[info.Name] = tool
	}
	return registry, nil
}

func (r *NativeRegistry) Tools() []NativeTool {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]NativeTool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *NativeRegistry) BaseTools() []einotool.BaseTool {
	tools := r.Tools()
	out := make([]einotool.BaseTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool)
	}
	return out
}

func (r *NativeRegistry) ToolInfos(ctx context.Context) ([]*schema.ToolInfo, error) {
	tools := r.Tools()
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		info, err := tool.Info(ctx)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}
