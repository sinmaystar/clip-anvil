package production

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
)

type fakeProviderStagingStore struct {
	key         string
	content     []byte
	contentType string
	url         string
}

func (s *fakeProviderStagingStore) PutObject(ctx context.Context, key string, body io.Reader, contentType string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.key = key
	s.content = data
	s.contentType = contentType
	return nil
}

func (s *fakeProviderStagingStore) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return s.url, nil
}

type fakeProviderSourceStore struct {
	workspaceID pgtype.UUID
	key         string
	url         string
}

func (s *fakeProviderSourceStore) PresignedGetURL(_ context.Context, workspaceID pgtype.UUID, key string, _ time.Duration) (string, error) {
	s.workspaceID = workspaceID
	s.key = key
	return s.url, nil
}

func TestTOSProviderAssetResolverStagesInputImageAndReturnsSignedURL(t *testing.T) {
	store := &fakeProviderStagingStore{url: "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com/provider-inputs/ws/job/node.png?X-Tos-Signature=test"}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://minio.local/workspace-a/input.png" {
			t.Fatalf("download url = %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(onePixelPNG)),
		}, nil
	})}
	resolver := NewTOSProviderAssetResolver(store, httpClient, TOSProviderAssetResolverConfig{
		PublicBaseURL: "http://minio.local",
		URLTTL:        time.Hour,
	})

	intent := videoIntent()
	intent.InputRefs[0].StorageURL = "workspace-a/input.png"
	resolved, err := resolver.ResolveInputRefs(context.Background(), ProductionJob{}, intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.InputRefs) != 1 {
		t.Fatalf("input refs = %#v", resolved.InputRefs)
	}
	if resolved.InputRefs[0].StorageURL != store.url {
		t.Fatalf("resolved storage url = %q", resolved.InputRefs[0].StorageURL)
	}
	if store.key == "" || store.contentType != "image/png" || !bytes.Equal(store.content, onePixelPNG) {
		t.Fatalf("staged key/type/content = %q/%q/%d", store.key, store.contentType, len(store.content))
	}
}

func TestTOSProviderAssetResolverUsesSourcePresignedURLForPrivateStorage(t *testing.T) {
	store := &fakeProviderStagingStore{url: "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com/provider-inputs/ws/job/node.png?X-Tos-Signature=test"}
	source := &fakeProviderSourceStore{url: "http://minio.local/presigned/input.png"}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != source.url {
			t.Fatalf("download url = %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(onePixelPNG)),
		}, nil
	})}
	resolver := NewTOSProviderAssetResolver(store, httpClient, TOSProviderAssetResolverConfig{
		URLTTL: time.Hour,
	}, source)
	workspaceID := pgtype.UUID{Bytes: [16]byte{0xaa, 0xbb, 0xcc, 0xdd}, Valid: true}
	intent := videoIntent()
	intent.InputRefs[0].StorageURL = storage.StorageURL(workspaceID, "input.png")

	_, err := resolver.ResolveInputRefs(context.Background(), ProductionJob{WorkspaceID: workspaceID}, intent)
	if err != nil {
		t.Fatal(err)
	}
	if source.key != "input.png" {
		t.Fatalf("source key = %q", source.key)
	}
	if source.workspaceID != workspaceID {
		t.Fatalf("workspace id = %#v", source.workspaceID)
	}
}

func TestTOSProviderAssetResolverStagesImageWhenResponseContentTypeIsGenericBinary(t *testing.T) {
	store := &fakeProviderStagingStore{url: "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com/provider-inputs/ws/job/node.png?X-Tos-Signature=test"}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"binary/octet-stream"}},
			Body:       io.NopCloser(bytes.NewReader(onePixelPNG)),
		}, nil
	})}
	resolver := NewTOSProviderAssetResolver(store, httpClient, TOSProviderAssetResolverConfig{
		PublicBaseURL: "http://minio.local",
		URLTTL:        time.Hour,
	})

	intent := videoIntent()
	intent.InputRefs[0].StorageURL = "workspace-a/input.png"
	_, err := resolver.ResolveInputRefs(context.Background(), ProductionJob{}, intent)
	if err != nil {
		t.Fatal(err)
	}
	if store.contentType != "image/png" || !bytes.Equal(store.content, onePixelPNG) {
		t.Fatalf("staged type/content = %q/%d", store.contentType, len(store.content))
	}
}
