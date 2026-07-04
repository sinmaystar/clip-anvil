package composer

import (
	"strings"
	"testing"
)

func TestComposerSystemPromptContainsSkillLibrary(t *testing.T) {
	for _, required := range []string{
		"ClipAnvil Composer",
		"## Skills Library",
		"load_agent_skill",
		"composer-timeline-director",
		"available_composition_assets",
		"still",
		"optional existing Seedance clips",
		"mixed-cost",
		"Composer does not call Seedance",
		"shot_04.preview_image.r1.node",
		"media_node",
	} {
		if !strings.Contains(SystemPrompt, required) {
			t.Fatalf("composer prompt missing %q", required)
		}
	}
	if strings.Contains(SystemPrompt, "# Composer Timeline Director") {
		t.Fatalf("composer prompt leaked skill body")
	}
}
