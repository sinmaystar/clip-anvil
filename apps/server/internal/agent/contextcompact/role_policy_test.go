package contextcompact

import "testing"

func TestM93RoleNamesAreStable(t *testing.T) {
	for _, role := range []string{"producer", "craftsman", "reviewer", "composer"} {
		if safePathPart(role) != role {
			t.Fatalf("role %q is not stable for compaction paths", role)
		}
	}
}
