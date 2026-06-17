package sandbox

import "testing"

func TestValidateOutputPathAcceptsOutputFile(t *testing.T) {
	path, err := ValidateOutputPath("/workspace/output/result.mp4")
	if err != nil {
		t.Fatalf("ValidateOutputPath error = %v", err)
	}
	if path != "/workspace/output/result.mp4" {
		t.Fatalf("path = %q", path)
	}
}

func TestValidateOutputPathRejectsEscape(t *testing.T) {
	for _, input := range []string{"/workspace/output/../secret", "/workspace/assets/a.png", "result.mp4", ""} {
		if _, err := ValidateOutputPath(input); err == nil {
			t.Fatalf("expected reject for %q", input)
		}
	}
}
