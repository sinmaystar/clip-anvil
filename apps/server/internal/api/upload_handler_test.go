package api

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestMediaTypeForMIMEAcceptsSupportedAssetTypes(t *testing.T) {
	testCases := []struct {
		mime      string
		mediaType db.AssetType
	}{
		{mime: "image/png", mediaType: db.AssetTypeImage},
		{mime: "image/jpeg", mediaType: db.AssetTypeImage},
		{mime: "video/mp4", mediaType: db.AssetTypeVideo},
		{mime: "audio/mpeg", mediaType: db.AssetTypeAudio},
	}

	for _, tc := range testCases {
		t.Run(tc.mime, func(t *testing.T) {
			mediaType, ok := mediaTypeForMIME(tc.mime)
			if !ok {
				t.Fatalf("%s should be supported", tc.mime)
			}
			if mediaType != tc.mediaType {
				t.Fatalf("media type = %q, want %q", mediaType, tc.mediaType)
			}
		})
	}
}

func TestMediaTypeForMIMERejectsText(t *testing.T) {
	if _, ok := mediaTypeForMIME("text/plain"); ok {
		t.Fatal("text/plain should not be accepted as a media asset")
	}
}

func TestSafeFilenameKeepsBasenameAndRemovesUnsafeCharacters(t *testing.T) {
	got := safeFilename("../产品 图.png")
	if got != "----.png" {
		t.Fatalf("safe filename = %q, want %q", got, "----.png")
	}
}

func TestSafeFilenameFallsBackForBlankName(t *testing.T) {
	if got := safeFilename(" "); got != "upload.bin" {
		t.Fatalf("safe filename = %q, want upload.bin", got)
	}
}
