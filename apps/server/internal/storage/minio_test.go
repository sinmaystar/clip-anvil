package storage

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

func TestStorageURLUsesWorkspaceBucket(t *testing.T) {
	workspaceID := testWorkspaceID()
	got := StorageURL(workspaceID, "assets/input.mp4")
	want := "workspace-aabbccdd-0000-0000-0000-000000000000/assets/input.mp4"
	if got != want {
		t.Fatalf("StorageURL() = %q, want %q", got, want)
	}
}

func TestPresignedSandboxURLUsesSandboxEndpoint(t *testing.T) {
	service, err := New(config.MinIOConfig{
		Endpoint:        "localhost:9000",
		SandboxEndpoint: "host.docker.internal:9000",
		AccessKey:       "clipanvil",
		SecretKey:       "clipanvil_dev",
		UseSSL:          false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rawURL, err := service.PresignedSandboxGetURL(context.Background(), testWorkspaceID(), "assets/input.mp4", time.Hour)
	if err != nil {
		t.Fatalf("PresignedSandboxGetURL error = %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse presigned url: %v", err)
	}
	if parsed.Host != "host.docker.internal:9000" {
		t.Fatalf("presigned host = %q, want host.docker.internal:9000", parsed.Host)
	}
	if parsed.Path != "/workspace-aabbccdd-0000-0000-0000-000000000000/assets/input.mp4" {
		t.Fatalf("presigned path = %q", parsed.Path)
	}
}

func testWorkspaceID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0xaa, 0xbb, 0xcc, 0xdd}, Valid: true}
}
