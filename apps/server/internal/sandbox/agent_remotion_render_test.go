package sandbox

import (
	"strings"
	"testing"
)

func TestAgentRemotionRenderCommandUsesFixedRuntime(t *testing.T) {
	command := agentRemotionRenderCommand("/workspace/agent-remotion/artifact-1/1", "/workspace/output/final.mp4")
	for _, want := range []string{
		"node /opt/clipanvil/remotion-agent-runtime/src/render.mjs",
		"--workdir '/workspace/agent-remotion/artifact-1/1'",
		"--out '/workspace/output/final.mp4'",
		"--browser-executable '/usr/bin/chromium-headless-shell'",
		"--public-dir '/workspace'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q: %s", want, command)
		}
	}
}
