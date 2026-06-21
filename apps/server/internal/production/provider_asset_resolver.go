package production

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
)

const defaultProviderInputMaxBytes = 20 << 20

type ProviderInputResolver interface {
	ResolveInputRefs(ctx context.Context, job ProductionJob, intent GenerationIntent) (GenerationIntent, error)
}

type providerStagingStore interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentType string) error
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type providerSourceStore interface {
	PresignedGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
}

type TOSProviderAssetResolverConfig struct {
	PublicBaseURL string
	URLTTL        time.Duration
	SourceURLTTL  time.Duration
}

type TOSProviderAssetResolver struct {
	store       providerStagingStore
	sourceStore providerSourceStore
	httpClient  *http.Client
	cfg         TOSProviderAssetResolverConfig
}

func NewTOSProviderAssetResolver(store providerStagingStore, httpClient *http.Client, cfg TOSProviderAssetResolverConfig, sourceStores ...providerSourceStore) *TOSProviderAssetResolver {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if cfg.URLTTL <= 0 {
		cfg.URLTTL = time.Hour
	}
	if cfg.SourceURLTTL <= 0 {
		cfg.SourceURLTTL = 15 * time.Minute
	}
	var sourceStore providerSourceStore
	if len(sourceStores) > 0 {
		sourceStore = sourceStores[0]
	}
	return &TOSProviderAssetResolver{store: store, sourceStore: sourceStore, httpClient: httpClient, cfg: cfg}
}

func (r *TOSProviderAssetResolver) ResolveInputRefs(ctx context.Context, job ProductionJob, intent GenerationIntent) (GenerationIntent, error) {
	if r == nil || r.store == nil {
		return intent, nil
	}
	resolved := intent
	resolved.InputRefs = append([]InputRef(nil), intent.InputRefs...)
	for index, ref := range resolved.InputRefs {
		if ref.NodeType != "image" || strings.TrimSpace(ref.StorageURL) == "" {
			continue
		}
		sourceURL, err := r.providerInputSourceURL(ctx, job.WorkspaceID, ref.StorageURL)
		if err != nil {
			return GenerationIntent{}, err
		}
		content, mime, err := downloadProviderAsset(ctx, r.httpClient, sourceURL, defaultProviderInputMaxBytes, allowedImageMIMEs())
		if err != nil {
			return GenerationIntent{}, fmt.Errorf("%w: stage provider input image: %v", ErrProviderExecution, err)
		}
		key := providerInputObjectKey(job, ref, mime)
		if err := r.store.PutObject(ctx, key, bytes.NewReader(content), mime); err != nil {
			return GenerationIntent{}, fmt.Errorf("%w: upload provider input image to tos: %v", ErrProviderExecution, err)
		}
		signedURL, err := r.store.PresignedGetURL(ctx, key, r.cfg.URLTTL)
		if err != nil {
			return GenerationIntent{}, fmt.Errorf("%w: sign provider input image url: %v", ErrProviderExecution, err)
		}
		resolved.InputRefs[index].StorageURL = signedURL
	}
	return resolved, nil
}

func (r *TOSProviderAssetResolver) providerInputSourceURL(ctx context.Context, workspaceID pgtype.UUID, storageURL string) (string, error) {
	storageURL = strings.TrimSpace(storageURL)
	if strings.HasPrefix(storageURL, "http://") || strings.HasPrefix(storageURL, "https://") {
		return storageURL, nil
	}
	if r.sourceStore != nil && workspaceID.Valid {
		key, err := storage.KeyFromStorageURL(workspaceID, storageURL)
		if err != nil {
			return "", fmt.Errorf("%w: resolve provider input source key: %v", ErrProviderExecution, err)
		}
		signedURL, err := r.sourceStore.PresignedGetURL(ctx, workspaceID, key, r.cfg.SourceURLTTL)
		if err != nil {
			return "", fmt.Errorf("%w: sign provider input source url: %v", ErrProviderExecution, err)
		}
		return signedURL, nil
	}
	return providerInputSourceURL(r.cfg.PublicBaseURL, storageURL), nil
}

func providerInputSourceURL(publicBaseURL string, storageURL string) string {
	storageURL = strings.TrimSpace(storageURL)
	if strings.HasPrefix(storageURL, "http://") || strings.HasPrefix(storageURL, "https://") {
		return storageURL
	}
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if publicBaseURL == "" {
		return storageURL
	}
	return publicBaseURL + "/" + strings.TrimLeft(storageURL, "/")
}

func providerInputObjectKey(job ProductionJob, ref InputRef, mime string) string {
	ext := extensionForMIME(mime)
	if ext == ".bin" {
		ext = path.Ext(ref.StorageURL)
	}
	return fmt.Sprintf("provider-inputs/%s/%s/%s%s", uuidToString(job.WorkspaceID), uuidToString(job.ID), uuidToString(ref.NodeID), ext)
}
