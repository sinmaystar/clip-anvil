package sandbox

import "testing"

func TestValidateWorkspaceTextPathRejectsEscape(t *testing.T) {
	for _, input := range []string{"", "../secret", "/tmp/x", "/workspace/../etc/passwd", "/workspace"} {
		if _, err := ValidateWorkspaceTextPath(input); err == nil {
			t.Fatalf("ValidateWorkspaceTextPath(%q) returned nil error", input)
		}
	}
}

func TestValidateWorkspaceTextPathAcceptsWorkspacePath(t *testing.T) {
	got, err := ValidateWorkspaceTextPath("/workspace/.clipanvil/notes/plan.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/.clipanvil/notes/plan.md" {
		t.Fatalf("path = %q, want /workspace/.clipanvil/notes/plan.md", got)
	}
}

func TestApplyTextEditCreateModes(t *testing.T) {
	got, err := ApplyTextEdit("", TextEditInput{Mode: "create", Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "first" {
		t.Fatalf("create result = %q, want first", got)
	}
	got, err = ApplyTextEdit("old", TextEditInput{Mode: "create_or_overwrite", Content: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "new" {
		t.Fatalf("overwrite result = %q, want new", got)
	}
}

func TestApplyTextEditAppend(t *testing.T) {
	got, err := ApplyTextEdit("old", TextEditInput{Mode: "append", Content: "\nnew"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "old\nnew" {
		t.Fatalf("append result = %q, want old newline new", got)
	}
}

func TestApplyTextEditReplaceRequiresUniqueMatch(t *testing.T) {
	if _, err := ApplyTextEdit("one one", TextEditInput{Mode: "replace", OldText: "one", NewText: "two"}); err == nil {
		t.Fatal("expected non-unique old_text to fail")
	}
	if _, err := ApplyTextEdit("one", TextEditInput{Mode: "replace", OldText: "missing", NewText: "two"}); err == nil {
		t.Fatal("expected missing old_text to fail")
	}
	got, err := ApplyTextEdit("one", TextEditInput{Mode: "replace", OldText: "one", NewText: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "two" {
		t.Fatalf("replace result = %q, want two", got)
	}
}
