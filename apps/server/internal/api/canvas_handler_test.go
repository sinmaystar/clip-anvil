package api

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestToCameraResponseMapsCanvasDocument(t *testing.T) {
	camera := toCameraResponse(db.CanvasDocument{
		CameraX:    12.5,
		CameraY:    -8,
		CameraZoom: 1.25,
	})

	if camera.X != 12.5 {
		t.Fatalf("camera x = %v, want %v", camera.X, 12.5)
	}
	if camera.Y != -8 {
		t.Fatalf("camera y = %v, want %v", camera.Y, -8)
	}
	if camera.Zoom != 1.25 {
		t.Fatalf("camera zoom = %v, want %v", camera.Zoom, 1.25)
	}
}
