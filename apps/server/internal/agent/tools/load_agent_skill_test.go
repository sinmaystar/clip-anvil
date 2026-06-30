package tools

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	agentskills "github.com/sinmaystar/clip-anvil/internal/agent/skills"
)

func TestLoadAgentSkillInfo(t *testing.T) {
	tool := NewLoadAgentSkillNativeTool(nil, agentskills.RoleProducer)
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "load_agent_skill" {
		t.Fatalf("tool name = %q", info.Name)
	}
	if !strings.Contains(info.Desc, "加载") {
		t.Fatalf("tool desc = %q", info.Desc)
	}
}

func TestLoadAgentSkillReturnsContentForAllowedRoleAndTask(t *testing.T) {
	registry := testSkillRegistry(t)
	tool := NewLoadAgentSkillNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"commerce-ad-producer","reason":"需要制定电商视频生产策略"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name":"commerce-ad-producer"`, `"version":"0.1.0"`, `"source_hash":"sha256:`, "# Producer Body"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLoadAgentSkillRequiresReason(t *testing.T) {
	registry := testSkillRegistry(t)
	tool := NewLoadAgentSkillNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"commerce-ad-producer"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "reason") || !strings.Contains(out, "工具调用失败") {
		t.Fatalf("output = %s", out)
	}
}

func TestLoadAgentSkillSectionsReturnWarning(t *testing.T) {
	registry := testSkillRegistry(t)
	tool := NewLoadAgentSkillNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"commerce-ad-producer","reason":"需要指定章节","sections":["Tool Protocol"]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"warnings", "M8.1 暂不支持按 sections 裁剪", "# Producer Body"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLoadAgentSkillRejectsWrongRole(t *testing.T) {
	registry := testSkillRegistry(t)
	tool := NewLoadAgentSkillNativeTool(registry, agentskills.RoleReviewer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("reviewer_turn"), `{"name":"commerce-ad-producer","reason":"Reviewer 不应读取 Producer skill"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "当前角色不能加载该 skill") {
		t.Fatalf("output = %s", out)
	}
	if strings.Contains(out, "# Producer Body") {
		t.Fatalf("wrong-role output leaked body:\n%s", out)
	}
}

func TestLoadAgentSkillRejectsWrongTaskType(t *testing.T) {
	registry := testSkillRegistry(t)
	tool := NewLoadAgentSkillNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("composer_turn"), `{"name":"commerce-ad-producer","reason":"错误 task type"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "当前任务类型不能加载该 skill") {
		t.Fatalf("output = %s", out)
	}
}

func TestLoadAgentSkillHandlesMissingSkill(t *testing.T) {
	registry := testSkillRegistry(t)
	tool := NewLoadAgentSkillNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"missing-skill","reason":"测试缺失 skill"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "未找到该 skill") {
		t.Fatalf("output = %s", out)
	}
}

func testSkillRegistry(t *testing.T) *agentskills.Registry {
	t.Helper()
	registry, err := agentskills.NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
version: 0.1.0
---
# Producer Body
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func contextWithSkillRuntime(taskType string) context.Context {
	return WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		TaskType:    taskType,
		ToolCallID:  "call_skill",
	})
}
