package tools

import (
	"encoding/json"
	"strings"
	"testing"

	agentskills "github.com/sinmaystar/clip-anvil/internal/agent/skills"
)

func TestM8SkillRuntimeEndToEndAcrossRoles(t *testing.T) {
	registry := agentskills.DefaultRegistry()
	cases := []struct {
		role     agentskills.Role
		taskType string
		skill    string
		bodyWant string
	}{
		{role: agentskills.RoleProducer, taskType: "producer_turn", skill: "commerce-ad-producer", bodyWant: "## Quality Bar"},
		{role: agentskills.RoleCraftsman, taskType: "craftsman_turn", skill: "seedance-renderplan-craftsman", bodyWant: "model_prompt_profile"},
		{role: agentskills.RoleReviewer, taskType: "reviewer_turn", skill: "reviewer-quality-gate", bodyWant: "structured review result"},
		{role: agentskills.RoleComposer, taskType: "composer_turn", skill: "composer-timeline-director", bodyWant: "TimelinePlan"},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			block := agentskills.PromptBlock(registry, tc.role)
			if !strings.Contains(block, "## Skills Library") || !strings.Contains(block, tc.skill) {
				t.Fatalf("prompt block for %s missing skill metadata:\n%s", tc.role, block)
			}
			if strings.Contains(block, "## Quality Bar") {
				t.Fatalf("prompt block leaked skill body for %s:\n%s", tc.role, block)
			}

			tool := NewLoadAgentSkillNativeTool(registry, tc.role)
			raw, err := tool.InvokableRun(contextWithSkillRuntime(tc.taskType), `{"name":"`+tc.skill+`","reason":"M8 E2E loads role skill"}`)
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				Name       string   `json:"name"`
				Version    string   `json:"version"`
				SourceHash string   `json:"source_hash"`
				RoleScope  []string `json:"role_scope"`
				Content    string   `json:"content"`
			}
			if err := json.Unmarshal([]byte(raw), &out); err != nil {
				t.Fatalf("decode output %s: %v", raw, err)
			}
			if out.Name != tc.skill || out.Version == "" || !strings.HasPrefix(out.SourceHash, "sha256:") {
				t.Fatalf("bad loaded skill output: %#v", out)
			}
			if !strings.Contains(out.Content, tc.bodyWant) {
				t.Fatalf("%s content missing %q:\n%s", tc.skill, tc.bodyWant, out.Content)
			}
		})
	}
}

func TestM8SkillRuntimeEndToEndLoadsControlledResource(t *testing.T) {
	registry := agentskills.DefaultRegistry()
	tool := NewLoadAgentSkillResourceNativeTool(registry, agentskills.RoleProducer)
	raw, err := tool.InvokableRun(contextWithSkillRuntime("producer_turn"), `{"name":"commerce-ad-producer","resource_path":"references/checklist.md","reason":"M8 E2E loads controlled resource"}`)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Name         string `json:"name"`
		ResourcePath string `json:"resource_path"`
		Version      string `json:"version"`
		SourceHash   string `json:"source_hash"`
		Content      string `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode output %s: %v", raw, err)
	}
	if out.Name != "commerce-ad-producer" || out.ResourcePath != "references/checklist.md" || out.Version == "" || !strings.HasPrefix(out.SourceHash, "sha256:") {
		t.Fatalf("bad resource output: %#v", out)
	}
	if !strings.Contains(out.Content, "Confirm CreativeBrief") {
		t.Fatalf("resource content = %s", out.Content)
	}
}
