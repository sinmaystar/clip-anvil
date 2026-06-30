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

const toolLoadAgentSkill = "load_agent_skill"

type SkillLoader interface {
	Load(name string, role agentskills.Role, taskType string) (agentskills.LoadedSkill, error)
	SkillsForRole(role agentskills.Role) []agentskills.SkillMetadata
}

type LoadAgentSkillNativeTool struct {
	registry SkillLoader
	role     agentskills.Role
}

type LoadAgentSkillInput struct {
	Name     string   `json:"name" jsonschema:"required" jsonschema_description:"要加载的 skill name，必须来自当前 system prompt 的 Available Skills 列表。"`
	Reason   string   `json:"reason" jsonschema:"required" jsonschema_description:"为什么当前任务需要加载该 skill，用于审计和调试。"`
	Sections []string `json:"sections" jsonschema_description:"可选章节名。M8.1 暂不做章节裁剪，会返回完整 SKILL.md 正文。"`
}

type loadAgentSkillOutput struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	SourceHash string   `json:"source_hash"`
	RoleScope  []string `json:"role_scope"`
	Content    string   `json:"content"`
	Warnings   []string `json:"warnings"`
}

func NewLoadAgentSkillNativeTool(registry SkillLoader, role agentskills.Role) *LoadAgentSkillNativeTool {
	return &LoadAgentSkillNativeTool{registry: registry, role: role}
}

func (t *LoadAgentSkillNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[LoadAgentSkillInput](
		toolLoadAgentSkill,
		"按 name 加载当前角色允许访问的 Agent Skill 正文。system prompt 只包含 skill name/description；执行相关计划或工具前，应先用本工具加载完整 SKILL.md。",
	)
}

func (t *LoadAgentSkillNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolLoadAgentSkill, raw, validateLoadAgentSkillInput)
	if !ok {
		return msg, nil
	}
	if t.registry == nil {
		return NaturalToolError(toolLoadAgentSkill, "skill registry 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolLoadAgentSkill)
	if !ok {
		return msg, nil
	}
	loaded, err := t.registry.Load(strings.TrimSpace(input.Name), t.role, strings.TrimSpace(runtime.TaskType))
	if err != nil {
		return t.naturalLoadError(err, input.Name), nil
	}
	roleScope := make([]string, 0, len(loaded.RoleScope))
	for _, role := range loaded.RoleScope {
		roleScope = append(roleScope, string(role))
	}
	warnings := []string{}
	if len(input.Sections) > 0 {
		warnings = append(warnings, "M8.1 暂不支持按 sections 裁剪，已返回完整 SKILL.md 正文。")
	}
	out, err := json.Marshal(loadAgentSkillOutput{
		Name:       loaded.Name,
		Version:    loaded.Version,
		SourceHash: loaded.SourceHash,
		RoleScope:  roleScope,
		Content:    loaded.Content,
		Warnings:   warnings,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *LoadAgentSkillNativeTool) naturalLoadError(err error, name string) string {
	available := t.availableSkillNames()
	switch {
	case errors.Is(err, agentskills.ErrSkillNotFound):
		return NaturalToolError(toolLoadAgentSkill, fmt.Sprintf("未找到该 skill：%s。", strings.TrimSpace(name)), "请只使用当前 system prompt Available Skills 中列出的 skill name。可用 skill："+available)
	case errors.Is(err, agentskills.ErrSkillRoleDenied):
		return NaturalToolError(toolLoadAgentSkill, fmt.Sprintf("当前角色不能加载该 skill：%s。", strings.TrimSpace(name)), "请选择当前角色 Available Skills 中列出的 skill，或让对应 Agent 处理该工作。可用 skill："+available)
	case errors.Is(err, agentskills.ErrSkillTaskDenied):
		return NaturalToolError(toolLoadAgentSkill, fmt.Sprintf("当前任务类型不能加载该 skill：%s。", strings.TrimSpace(name)), "请确认当前 Agent task 是否适合该 skill。可用 skill："+available)
	default:
		return NaturalToolError(toolLoadAgentSkill, err.Error(), "请修正 skill name 或稍后重试。")
	}
}

func (t *LoadAgentSkillNativeTool) availableSkillNames() string {
	if t.registry == nil {
		return "无"
	}
	skills := t.registry.SkillsForRole(t.role)
	if len(skills) == 0 {
		return "无"
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	return strings.Join(names, ", ")
}

func validateLoadAgentSkillInput(input LoadAgentSkillInput) error {
	if err := requireText(input.Name, "name"); err != nil {
		return err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return err
	}
	return nil
}
