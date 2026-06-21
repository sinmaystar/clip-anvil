package production

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"
)

type TOSStagingStoreConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Endpoint        string
	Region          string
	PublicBaseURL   string
}

type tosStagingClient interface {
	PutObjectV2(ctx context.Context, input *tos.PutObjectV2Input) (*tos.PutObjectV2Output, error)
	PreSignedURL(input *tos.PreSignedURLInput) (*tos.PreSignedURLOutput, error)
}

type TOSStagingStore struct {
	client        tosStagingClient
	bucket        string
	publicBaseURL string
}

func NewTOSStagingStore(cfg TOSStagingStoreConfig) (*TOSStagingStore, error) {
	if strings.TrimSpace(cfg.AccessKeyID) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_ACCESS_KEY_ID is required", ErrProviderConfig)
	}
	if strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_SECRET_ACCESS_KEY is required", ErrProviderConfig)
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_BUCKET is required", ErrProviderConfig)
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_ENDPOINT is required", ErrProviderConfig)
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_REGION is required", ErrProviderConfig)
	}
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_PUBLIC_BASE_URL is required", ErrProviderConfig)
	}
	client, err := tos.NewClientV2(
		strings.TrimSpace(cfg.Endpoint),
		tos.WithRegion(strings.TrimSpace(cfg.Region)),
		tos.WithCredentials(tos.NewStaticCredentials(strings.TrimSpace(cfg.AccessKeyID), strings.TrimSpace(cfg.SecretAccessKey))),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: create tos client: %v", ErrProviderConfig, err)
	}
	return &TOSStagingStore{client: client, bucket: strings.TrimSpace(cfg.Bucket), publicBaseURL: strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")}, nil
}

func newTOSStagingStoreForTest(client tosStagingClient, bucket string) *TOSStagingStore {
	return &TOSStagingStore{client: client, bucket: bucket}
}

func (s *TOSStagingStore) PutObject(ctx context.Context, key string, body io.Reader, contentType string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("%w: tos staging store is not configured", ErrProviderConfig)
	}
	_, err := s.client.PutObjectV2(ctx, &tos.PutObjectV2Input{
		PutObjectBasicInput: tos.PutObjectBasicInput{
			Bucket:      s.bucket,
			Key:         key,
			ContentType: contentType,
		},
		Content: body,
	})
	return err
}

func (s *TOSStagingStore) PresignedGetURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("%w: tos staging store is not configured", ErrProviderConfig)
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	isCustomDomain := true
	out, err := s.client.PreSignedURL(&tos.PreSignedURLInput{
		HTTPMethod:          enum.HttpMethodGet,
		Bucket:              s.bucket,
		Key:                 key,
		Expires:             int64(ttl / time.Second),
		AlternativeEndpoint: s.publicBaseURL,
		IsCustomDomain:      &isCustomDomain,
	})
	if err != nil {
		return "", err
	}
	return out.SignedUrl, nil
}
