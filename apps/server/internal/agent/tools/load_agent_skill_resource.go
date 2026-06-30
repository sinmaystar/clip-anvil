package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	agentskills "github.com/sinmaystar/clip-anvil/internal/agent/skills"
)

const toolLoadAgentSkillResource = "load_agent_skill_resource"

type SkillResourceLoader interface {
	LoadResource(name string, role agentskills.Role, taskType string, resourcePath string) (agentskills.LoadedSkillResource, error)
	SkillsForRole(role agentskills.Role) []agentskills.SkillMetadata
}

type LoadAgentSkillResourceNativeTool struct {
	registry SkillResourceLoader
	role     agentskills.Role
}

type LoadAgentSkillResourceInput struct {
	Name         string `json:"name" jsonschema:"required" jsonschema_description:"要加载资源所属的 skill name，必须来自当前角色可用 skill。"`
	ResourcePath string `json:"resource_path" jsonschema:"required" jsonschema_description:"skill 目录内的 markdown 资源路径，例如 references/checklist.md。只允许 .md 资源，不能越界。"`
	Reason       string `json:"reason" jsonschema:"required" jsonschema_description:"为什么当前任务需要加载该附加资源，用于审计和调试。"`
}

type loadAgentSkillResourceOutput struct {
	Name         string `json:"name"`
	ResourcePath string `json:"resource_path"`
	Version      string `json:"version"`
	SourceHash   string `json:"source_hash"`
	Content      string `json:"content"`
}

func NewLoadAgentSkillResourceNativeTool(registry SkillResourceLoader, role agentskills.Role) *LoadAgentSkillResourceNativeTool {
	return &LoadAgentSkillResourceNativeTool{registry: registry, role: role}
}

func (t *LoadAgentSkillResourceNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[LoadAgentSkillResourceInput](
		toolLoadAgentSkillResource,
		"加载当前角色允许访问的 Agent Skill 附加 markdown resource。只能读取 skill 目录内白名单 .md 资源，不执行脚本、不读取任意文件。",
	)
}

func (t *LoadAgentSkillResourceNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolLoadAgentSkillResource, raw, validateLoadAgentSkillResourceInput)
	if !ok {
		return msg, nil
	}
	if t.registry == nil {
		return NaturalToolError(toolLoadAgentSkillResource, "skill registry 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolLoadAgentSkillResource)
	if !ok {
		return msg, nil
	}
	loaded, err := t.registry.LoadResource(strings.TrimSpace(input.Name), t.role, strings.TrimSpace(runtime.TaskType), strings.TrimSpace(input.ResourcePath))
	if err != nil {
		return t.naturalLoadResourceError(err, input.Name), nil
	}
	out, err := json.Marshal(loadAgentSkillResourceOutput{
		Name:         loaded.Name,
		ResourcePath: loaded.ResourcePath,
		Version:      loaded.Version,
		SourceHash:   loaded.SourceHash,
		Content:      loaded.Content,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *LoadAgentSkillResourceNativeTool) naturalLoadResourceError(err error, name string) string {
	available := availableSkillNamesFor(t.registry, t.role)
	switch {
	case errors.Is(err, agentskills.ErrSkillNotFound):
		return NaturalToolError(toolLoadAgentSkillResource, fmt.Sprintf("未找到该 skill resource 所属 skill：%s。", strings.TrimSpace(name)), "请只使用当前角色可用 skill。可用 skill："+available)
	case errors.Is(err, agentskills.ErrSkillRoleDenied):
		return NaturalToolError(toolLoadAgentSkillResource, fmt.Sprintf("当前角色不能加载该 skill resource：%s。", strings.TrimSpace(name)), "请选择当前角色 Available Skills 中列出的 skill resource，或让对应 Agent 处理该工作。可用 skill："+available)
	case errors.Is(err, agentskills.ErrSkillTaskDenied):
		return NaturalToolError(toolLoadAgentSkillResource, fmt.Sprintf("当前任务类型不能加载该 skill resource：%s。", strings.TrimSpace(name)), "请确认当前 Agent task 是否适合该 skill resource。可用 skill："+available)
	case errors.Is(err, agentskills.ErrSkillResource):
		return NaturalToolError(toolLoadAgentSkillResource, fmt.Sprintf("不能加载该 skill resource：%s。", strings.TrimSpace(name)), "resource_path 只能是该 skill 目录内存在的 .md 文件，例如 references/checklist.md。")
	default:
		return NaturalToolError(toolLoadAgentSkillResource, err.Error(), "请修正 skill name 或 resource_path 后重试。")
	}
}

func validateLoadAgentSkillResourceInput(input LoadAgentSkillResourceInput) error {
	if err := requireText(input.Name, "name"); err != nil {
		return err
	}
	if err := requireText(input.ResourcePath, "resource_path"); err != nil {
		return err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return err
	}
	return nil
}

func availableSkillNamesFor(registry interface {
	SkillsForRole(role agentskills.Role) []agentskills.SkillMetadata
}, role agentskills.Role) string {
	if registry == nil {
		return "无"
	}
	skills := registry.SkillsForRole(role)
	if len(skills) == 0 {
		return "无"
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	return strings.Join(names, ", ")
}
