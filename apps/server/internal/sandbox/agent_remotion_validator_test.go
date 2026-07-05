package sandbox

import "testing"

func TestValidateAgentRemotionSnapshotAcceptsSafeRenderer(t *testing.T) {
	snapshot := validAgentRemotionSnapshot(t, map[string]string{
		"GeneratedComposition.tsx": `import React from "react";
import {AbsoluteFill, Img, staticFile} from "remotion";

export default function Video(props) {
  return <AbsoluteFill><Img src={staticFile(props.asset_manifest[0].path)} /></AbsoluteFill>;
}`,
	}, []byte(`{"output":{"width":1080,"height":1920,"fps":30,"duration_sec":6},"asset_manifest":[{"path":"input/product.png"}]}`))
	result := ValidateAgentRemotionSnapshot(snapshot)
	if !result.Passed || len(result.Errors) != 0 {
		t.Fatalf("validation failed: %#v", result)
	}
	if result.SourceHash != snapshot.SourceHash || result.PropsHash != snapshot.PropsHash {
		t.Fatalf("hashes not propagated: %#v", result)
	}
}

func TestValidateAgentRemotionSnapshotRejectsUnsafeRendererCode(t *testing.T) {
	cases := []struct {
		name string
		code string
		body string
	}{
		{"fs import", "forbidden_import", `import fs from "fs"; export default function Video(){ return null; }`},
		{"node builtin", "forbidden_import", `import cp from "node:child_process"; export default function Video(){ return null; }`},
		{"dynamic import", "dynamic_import", `export default async function Video(){ await import("fs"); return null; }`},
		{"require", "require_call", `const fs = require("fs"); export default function Video(){ return null; }`},
		{"fetch", "network_api", `export default function Video(){ fetch("https://example.com"); return null; }`},
		{"external url", "external_url", `export default function Video(){ return "https://example.com/a.png"; }`},
		{"eval", "eval_call", `export default function Video(){ eval("1+1"); return null; }`},
		{"function constructor", "function_constructor", `export default function Video(){ return Function("return 1")(); }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validAgentRemotionSnapshot(t, map[string]string{"GeneratedComposition.tsx": tc.body}, []byte(`{"output":{"fps":30}}`))
			result := ValidateAgentRemotionSnapshot(snapshot)
			if result.Passed {
				t.Fatalf("expected validation to fail for %s", tc.name)
			}
			if !hasAgentRemotionValidationCode(result, tc.code) {
				t.Fatalf("missing code %q in %#v", tc.code, result.Errors)
			}
		})
	}
}

func TestValidateAgentRemotionSnapshotRejectsInvalidPropsJSON(t *testing.T) {
	snapshot := validAgentRemotionSnapshot(t, map[string]string{
		"GeneratedComposition.tsx": `export default function Video(){ return null; }`,
	}, []byte(`{"output":`))
	result := ValidateAgentRemotionSnapshot(snapshot)
	if result.Passed || !hasAgentRemotionValidationCode(result, "invalid_props_json") {
		t.Fatalf("expected invalid props JSON error, got %#v", result)
	}
}

func TestValidateAgentRemotionSnapshotRequiresGeneratedComposition(t *testing.T) {
	snapshot := validAgentRemotionSnapshot(t, map[string]string{
		"styles.ts": `export const color = "#fff";`,
	}, []byte(`{"output":{"fps":30}}`))
	result := ValidateAgentRemotionSnapshot(snapshot)
	if result.Passed || !hasAgentRemotionValidationCode(result, "missing_generated_composition") {
		t.Fatalf("expected missing generated composition error, got %#v", result)
	}
}

func validAgentRemotionSnapshot(t *testing.T, files map[string]string, propsJSON []byte) AgentRemotionSnapshot {
	t.Helper()
	snapshot, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", files, propsJSON)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func hasAgentRemotionValidationCode(result AgentRemotionValidationResult, code string) bool {
	for _, issue := range result.Errors {
		if issue.Code == code {
			return true
		}
	}
	return false
}
