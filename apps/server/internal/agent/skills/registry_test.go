package skills

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRegistryParsesFrontmatterAndFiltersByRole(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
version: 0.1.0
---
# Producer Body
`)},
		"library/craftsman/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: seedance-renderplan-craftsman
description: Use when Craftsman writes a Seedance RenderPlan.
role_scope: [craftsman]
task_types: [craftsman_turn]
version: 0.1.0
---
# Craftsman Body
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	producerSkills := registry.SkillsForRole(RoleProducer)
	if len(producerSkills) != 1 || producerSkills[0].Name != "commerce-ad-producer" {
		t.Fatalf("producer skills = %#v", producerSkills)
	}
	craftsmanSkills := registry.SkillsForRole(RoleCraftsman)
	if len(craftsmanSkills) != 1 || craftsmanSkills[0].Name != "seedance-renderplan-craftsman" {
		t.Fatalf("craftsman skills = %#v", craftsmanSkills)
	}
}

func TestPromptBlockIncludesOnlyMetadata(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn]
version: 0.1.0
---
# Producer Body

Detailed body must not be in system prompt.
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	block := PromptBlock(registry, RoleProducer)
	for _, want := range []string{"## Skills Library", "load_agent_skill", "commerce-ad-producer", "Use when Producer plans a commerce ad."} {
		if !strings.Contains(block, want) {
			t.Fatalf("prompt block missing %q:\n%s", want, block)
		}
	}
	for _, forbidden := range []string{"# Producer Body", "Detailed body must not be in system prompt."} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("prompt block leaked body %q:\n%s", forbidden, block)
		}
	}
}

func TestLoadSkillValidatesRoleAndTaskType(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn]
version: 0.1.0
---
# Producer Body
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := registry.Load("commerce-ad-producer", RoleProducer, "producer_turn")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != "0.1.0" || loaded.SourceHash == "" || !strings.Contains(loaded.Content, "# Producer Body") {
		t.Fatalf("loaded = %#v", loaded)
	}
	if _, err := registry.Load("commerce-ad-producer", RoleReviewer, "reviewer_turn"); err == nil {
		t.Fatal("expected wrong role to be rejected")
	}
	if _, err := registry.Load("commerce-ad-producer", RoleProducer, "composer_turn"); err == nil {
		t.Fatal("expected wrong task type to be rejected")
	}
}

func TestM82DefaultRegistryContainsCommerceSkillPack(t *testing.T) {
	registry := DefaultRegistry()
	expected := map[Role][]string{
		RoleProducer: {
			"audio-plan-producer",
			"commerce-ad-producer",
			"hitl-checkpoint-producer",
			"motion-shot-producer",
			"reference-video-analysis-producer",
		},
		RoleCraftsman: {
			"audio-renderplan-craftsman",
			"motion-shot-craftsman",
			"renderplan-repair-craftsman",
			"seedance-renderplan-craftsman",
			"seedream-renderplan-craftsman",
		},
		RoleReviewer: {
			"commerce-delivery-promise-reviewer",
			"final-video-audio-reviewer",
			"final-video-remotion-reviewer",
			"motion-shot-reviewer",
			"reference-consistency-reviewer",
			"reviewer-quality-gate",
		},
		RoleComposer: {
			"composer-blocker-escalation",
			"composer-timeline-director",
			"ffmpeg-audio-mix-composer",
			"platform-export-composer",
			"remotion-timeline-composer",
		},
	}
	for role, want := range expected {
		gotSkills := registry.SkillsForRole(role)
		got := make([]string, 0, len(gotSkills))
		for _, skill := range gotSkills {
			got = append(got, skill.Name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s skills = %#v, want %#v", role, got, want)
		}
	}
}

func TestM82DefaultSkillsHaveRequiredSectionsAndAllowedTools(t *testing.T) {
	registry := DefaultRegistry()
	allowedTools := map[Role]map[string]bool{
		RoleProducer: boolSet(
			"read_project_context",
			"analyze_reference_video",
			"upsert_project_brief",
			"update_project_memory",
			"upsert_key_elements",
			"upsert_storyboard",
			"upsert_audio_plan",
			"dispatch_craftsman",
			"dispatch_composer",
			"decide_render_plan",
			"dispatch_reviewer",
			"request_user_decision",
		),
		RoleCraftsman: boolSet("read_project_memory", "upsert_render_plan"),
		RoleReviewer:  boolSet("read_project_context", "read_project_memory", "submit_review_result"),
		RoleComposer: boolSet(
			"get_composition_context",
			"stage_media_inputs",
			"probe_media",
			"create_timeline_plan",
			"update_timeline_plan_status",
			"render_timeline_template",
			"run_ffmpeg_command",
			"submit_composition_artifact",
		),
	}
	for role := range allowedTools {
		for _, meta := range registry.SkillsForRole(role) {
			loaded, err := registry.Load(meta.Name, role, "")
			if err != nil {
				t.Fatalf("load %s: %v", meta.Name, err)
			}
			for _, section := range []string{"## Use When", "## Do", "## Do Not", "## Tool Protocol", "## Quality Bar"} {
				if !strings.Contains(loaded.Content, section) {
					t.Fatalf("%s missing section %q", meta.Name, section)
				}
			}
			for _, tool := range loaded.Tools {
				if !allowedTools[role][tool] {
					t.Fatalf("%s references disallowed tool %q for role %s", meta.Name, tool, role)
				}
			}
		}
	}
}

func TestFinalVideoAudioReviewerSkillNamesAllFinalRequiredAxes(t *testing.T) {
	loaded, err := DefaultRegistry().Load("final-video-audio-reviewer", RoleReviewer, "reviewer_turn")
	if err != nil {
		t.Fatal(err)
	}
	for _, axis := range []string{
		"faithfulness",
		"brand_style_consistency",
		"visual_quality",
		"continuity",
		"audio_sync",
		"platform_selling_power",
	} {
		if !strings.Contains(loaded.Content, axis) {
			t.Fatalf("final-video-audio-reviewer missing required axis %q:\n%s", axis, loaded.Content)
		}
	}
}

func TestFinalVideoRemotionReviewerSkillNamesTimelineQualityGates(t *testing.T) {
	loaded, err := DefaultRegistry().Load("final-video-remotion-reviewer", RoleReviewer, "reviewer_turn")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"caption lane",
		"cue/asset sync",
		"no-Seedance",
		"mixed-cost",
		"Seedance video segment",
		"Remotion still segment",
		"layout diversity",
		"audio_sync",
		"platform_selling_power",
	} {
		if !strings.Contains(loaded.Content, required) {
			t.Fatalf("final-video-remotion-reviewer missing %q:\n%s", required, loaded.Content)
		}
	}
}

func TestMotionShotSkillsCarryRemotionGuidance(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want []string
	}{
		{
			name: "motion-shot-producer",
			role: RoleProducer,
			want: []string{"remotion_timeline_v1", "Seedream", "Volcengine", "motion_shot_video", "Do not require every shot to become `motion_shot_video`"},
		},
		{
			name: "motion-shot-craftsman",
			role: RoleCraftsman,
			want: []string{"model_prompt_profile: motion_shot_video", "image_to_motion_video", "text_layers", "visual_layers", "motion_style", "Do not write raw Remotion"},
		},
		{
			name: "motion-shot-reviewer",
			role: RoleReviewer,
			want: []string{"scene_timing", "text_safety", "product_visibility", "motion_rhythm", "audio_readiness", "seedance_policy", "pre_render_plan_review", "faithfulness", "subject_consistency", "continuity", "Do not invent rubric axis names"},
		},
	}
	for _, tt := range tests {
		loaded, err := DefaultRegistry().Load(tt.name, tt.role, "")
		if err != nil {
			t.Fatalf("load %s: %v", tt.name, err)
		}
		for _, want := range tt.want {
			if !strings.Contains(loaded.Content, want) {
				t.Fatalf("%s missing %q:\n%s", tt.name, want, loaded.Content)
			}
		}
	}
}

func TestCommerceAdProducerPreservesDynamicStoryboardForNoSeedance(t *testing.T) {
	loaded, err := DefaultRegistry().Load("commerce-ad-producer", RoleProducer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"no-Seedance does not reduce the storyboard to one shot",
		"20-45 second commerce ads usually need 4-9 shots",
		"each shot must have narrative_purpose, duration_sec, visual_intent, action_text, camera_intent, and narration",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("commerce-ad-producer missing %q\n%s", needle, loaded.Content)
		}
	}
}

func TestMotionShotProducerIsRoutePolicyOnly(t *testing.T) {
	loaded, err := DefaultRegistry().Load("motion-shot-producer", RoleProducer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"must be paired with commerce-ad-producer",
		"do not create a fixed storyboard",
		"dispatch Composer with `template_key=remotion_timeline_v1`",
		"Do not require every shot to become `motion_shot_video`",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("motion-shot-producer missing %q\n%s", needle, loaded.Content)
		}
	}
}

func TestMotionShotCraftsmanInheritsDynamicShotFacts(t *testing.T) {
	loaded, err := DefaultRegistry().Load("motion-shot-craftsman", RoleCraftsman, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"inherit the current shot duration_sec",
		"vary layout, motion_style, transitions, and text positions by shot purpose",
		"do not bake full voiceover subtitles into the motion shot",
		"block the task when no usable image input is available",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("motion-shot-craftsman missing %q\n%s", needle, loaded.Content)
		}
	}
}

func TestRemotionTimelineComposerSkillGuidesCueAlignedStillTimeline(t *testing.T) {
	loaded, err := DefaultRegistry().Load("remotion-timeline-composer", RoleComposer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"remotion_timeline_v1",
		"still images are first-class visual assets",
		"existing `clip` assets as video segments",
		"Use `type=image` for still assets and `type=video` for clip assets",
		"Composer only packages assets returned by `get_composition_context`",
		"AudioPlan cue_plan is the primary timing contract",
		"Do not use narrative_purpose, visual_intent, action_text, or camera_intent as captions",
		"match wheel cues to wheel/detail assets and storage cues to open-interior assets",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("remotion-timeline-composer missing %q\n%s", needle, loaded.Content)
		}
	}
}

func TestSeedanceCraftsmanRefusesMotionOnlyRoute(t *testing.T) {
	loaded, err := DefaultRegistry().Load("seedance-renderplan-craftsman", RoleCraftsman, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"video_route_policy=motion_only",
		"must not create seedance_2_video",
		"mark the task blocked or ask Producer to change the route",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("seedance-renderplan-craftsman missing %q\n%s", needle, loaded.Content)
		}
	}
}

func TestM82PromptBlockStillExcludesSkillBodies(t *testing.T) {
	block := PromptBlock(DefaultRegistry(), RoleCraftsman)
	if !strings.Contains(block, "seedance-renderplan-craftsman") || !strings.Contains(block, "seedream-renderplan-craftsman") {
		t.Fatalf("prompt block missing skill metadata:\n%s", block)
	}
	for _, forbidden := range []string{"## Tool Protocol", "## Quality Bar", "# Seedance RenderPlan Craftsman"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("prompt block leaked body %q:\n%s", forbidden, block)
		}
	}
}

func TestRegistryLoadsMarkdownResourceAndTracksUsage(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
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
	if _, err := registry.Load("commerce-ad-producer", RoleProducer, "producer_turn"); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.LoadResource("commerce-ad-producer", RoleProducer, "producer_turn", "references/checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if resource.ResourcePath != "references/checklist.md" || resource.SourceHash == "" || !strings.Contains(resource.Content, "Confirm durable facts") {
		t.Fatalf("resource = %#v", resource)
	}
	stats := registry.UsageStats()
	if !usageStatsContain(stats, "skill", "commerce-ad-producer", 1) {
		t.Fatalf("stats missing skill load: %#v", stats)
	}
	if !usageStatsContain(stats, "resource", "commerce-ad-producer/references/checklist.md", 1) {
		t.Fatalf("stats missing resource load: %#v", stats)
	}
}

func TestRegistryRejectsUnsafeSkillResourcePaths(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn]
version: 0.1.0
---
# Producer Body
`)},
		"library/producer/references/checklist.md": &fstest.MapFile{Data: []byte(`# Checklist`)},
		"library/producer/references/script.sh":    &fstest.MapFile{Data: []byte(`echo no`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../producer/SKILL.md", "/references/checklist.md", "references/script.sh", "missing.md"} {
		if _, err := registry.LoadResource("commerce-ad-producer", RoleProducer, "producer_turn", path); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestM84DefaultSkillToolReferencesMatchRoleRegistries(t *testing.T) {
	registry := DefaultRegistry()
	allowedTools := map[Role]map[string]bool{
		RoleProducer: boolSet(
			"load_agent_skill",
			"load_agent_skill_resource",
			"read_project_context",
			"analyze_reference_video",
			"upsert_project_brief",
			"update_project_memory",
			"upsert_key_elements",
			"upsert_storyboard",
			"upsert_audio_plan",
			"dispatch_craftsman",
			"dispatch_composer",
			"decide_render_plan",
			"dispatch_reviewer",
			"request_user_decision",
		),
		RoleCraftsman: boolSet("load_agent_skill", "load_agent_skill_resource", "read_project_memory", "upsert_render_plan"),
		RoleReviewer:  boolSet("load_agent_skill", "load_agent_skill_resource", "read_project_context", "read_project_memory", "submit_review_result"),
		RoleComposer: boolSet(
			"load_agent_skill",
			"load_agent_skill_resource",
			"get_composition_context",
			"stage_media_inputs",
			"probe_media",
			"create_timeline_plan",
			"update_timeline_plan_status",
			"render_timeline_template",
			"run_ffmpeg_command",
			"submit_composition_artifact",
		),
	}
	for role, allowed := range allowedTools {
		for _, skill := range registry.SkillsForRole(role) {
			for _, tool := range skill.Tools {
				if !allowed[tool] {
					t.Fatalf("%s references tool %q not present in %s registry", skill.Name, tool, role)
				}
			}
		}
	}
}

func boolSet(values ...string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func usageStatsContain(stats []UsageStat, kind string, name string, count int) bool {
	for _, stat := range stats {
		if stat.Kind == kind && stat.Name == name && stat.Count == count {
			return true
		}
	}
	return false
}
