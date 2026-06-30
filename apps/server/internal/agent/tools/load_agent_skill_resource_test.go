package tools

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	agentskills "github.com/sinmaystar/clip-anvil/internal/agent/skills"
)

func TestLoadAgentSkillResourceInfo(t *testing.T) {
	tool := NewLoadAgentSkillResourceNativeTool(nil, agentskills.RoleProducer)
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "load_agent_skill_resource" {
		t.Fatalf("tool name = %q", info.Name)
	}
	if !strings.Contains(info.Desc, "resource") && !strings.Contains(info.Desc, "资源") {
		t.Fatalf("tool desc = %q", info.Desc)
	}
}

func TestLoadAgentSkillResourceReturnsMarkdownForAllowedRoleAndTask(t *testing.T) {
	registry := testSkillResourceRegistry(t)
	tool := NewLoadAgentSkillResourceNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"commerce-ad-producer","resource_path":"references/checklist.md","reason":"需要详细检查清单"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name":"commerce-ad-producer"`, `"resource_path":"references/checklist.md"`, `"source_hash":"sha256:`, "Confirm durable facts"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLoadAgentSkillResourceRejectsWrongRoleWithoutLeakingContent(t *testing.T) {
	registry := testSkillResourceRegistry(t)
	tool := NewLoadAgentSkillResourceNativeTool(registry, agentskills.RoleReviewer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("reviewer_turn"), `{"name":"commerce-ad-producer","resource_path":"references/checklist.md","reason":"Reviewer 不应读取 Producer resource"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "当前角色不能加载该 skill resource") {
		t.Fatalf("output = %s", out)
	}
	if strings.Contains(out, "Confirm durable facts") {
		t.Fatalf("wrong-role output leaked content:\n%s", out)
	}
}

func TestLoadAgentSkillResourceRejectsPathTraversal(t *testing.T) {
	registry := testSkillResourceRegistry(t)
	tool := NewLoadAgentSkillResourceNativeTool(registry, agentskills.RoleProducer)
	out, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"commerce-ad-producer","resource_path":"../producer/SKILL.md","reason":"尝试越界"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "不能加载该 skill resource") {
		t.Fatalf("output = %s", out)
	}
}

func testSkillResourceRegistry(t *testing.T) *agentskills.Registry {
	t.Helper()
	registry, err := agentskills.NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn]
version: 0.1.0
---
# Producer Body
`)},
		"library/producer/references/checklist.md": &fstest.MapFile{Data: []byte(`# Checklist

- Confirm durable facts.
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
