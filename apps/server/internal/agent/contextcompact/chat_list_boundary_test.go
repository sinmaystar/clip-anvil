package contextcompact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactionCodeDoesNotUseAgentMessageUpdatePath(t *testing.T) {
	root := "."
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range []string{
			"UpdateAgentMessage",
			"UpdateMessage(",
			"ListAgentMessagesByThread",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("contextcompact production code must not use chat-list mutation/display path %q in %s", forbidden, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
