package api

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/auth"
)

func TestValidWorkspaceNameRejectsBlank(t *testing.T) {
	if validWorkspaceName("   ") {
		t.Fatal("blank workspace name must be invalid")
	}
}

func TestValidWorkspaceNameAcceptsTrimmedText(t *testing.T) {
	if !validWorkspaceName("  咖啡广告  ") {
		t.Fatal("non-blank workspace name must be valid")
	}
}

func TestAccountIDFromContextReadsMiddlewareValue(t *testing.T) {
	want := pgtype.UUID{
		Bytes: [16]byte{0x4a, 0x7b, 0x3c, 0x88, 0x90, 0x1d, 0x4c, 0xe7, 0xa1, 0x22, 0x5d, 0x10, 0x64, 0xee, 0xaa, 0x91},
		Valid: true,
	}
	c := &app.RequestContext{}
	c.Set(auth.AccountIDKey, want)

	got, ok := accountIDFromContext(c)
	if !ok {
		t.Fatal("expected account id to be present")
	}
	if got != want {
		t.Fatalf("account id = %v, want %v", got, want)
	}
}
