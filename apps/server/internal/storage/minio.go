package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

type Service struct {
	client        *minio.Client
	sandboxClient *minio.Client
}

type ObjectRef struct {
	Bucket     string
	Key        string
	StorageURL string
	Size       int64
	MIME       string
}

func New(cfg config.MinIOConfig) (*Service, error) {
	client, err := newClient(cfg.Endpoint, cfg)
	if err != nil {
		return nil, err
	}
	sandboxEndpoint := strings.TrimSpace(cfg.SandboxEndpoint)
	if sandboxEndpoint == "" {
		sandboxEndpoint = cfg.Endpoint
	}
	sandboxClient, err := newClient(sandboxEndpoint, cfg)
	if err != nil {
		return nil, err
	}
	return &Service{client: client, sandboxClient: sandboxClient}, nil
}

func NewWithClients(client *minio.Client, sandboxClient *minio.Client) *Service {
	if sandboxClient == nil {
		sandboxClient = client
	}
	return &Service{client: client, sandboxClient: sandboxClient}
}

func (s *Service) EnsureBucket(ctx context.Context, workspaceID pgtype.UUID) error {
	bucket := BucketName(workspaceID)
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
}

func (s *Service) ListBuckets(ctx context.Context) ([]minio.BucketInfo, error) {
	return s.client.ListBuckets(ctx)
}

func (s *Service) Upload(ctx context.Context, workspaceID pgtype.UUID, key string, reader io.Reader, size int64, contentType string) (ObjectRef, error) {
	if err := s.EnsureBucket(ctx, workspaceID); err != nil {
		return ObjectRef{}, err
	}
	key, err := CleanKey(key)
	if err != nil {
		return ObjectRef{}, err
	}
	bucket := BucketName(workspaceID)
	if _, err := s.client.PutObject(ctx, bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return ObjectRef{}, err
	}
	return ObjectRef{Bucket: bucket, Key: key, StorageURL: bucket + "/" + key, Size: size, MIME: contentType}, nil
}

func (s *Service) StatObject(ctx context.Context, workspaceID pgtype.UUID, key string) (ObjectRef, error) {
	key, err := CleanKey(key)
	if err != nil {
		return ObjectRef{}, err
	}
	bucket := BucketName(workspaceID)
	info, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectRef{}, err
	}
	return ObjectRef{Bucket: bucket, Key: key, StorageURL: bucket + "/" + key, Size: info.Size, MIME: info.ContentType}, nil
}

func (s *Service) PresignedGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	return s.presignedGetURL(ctx, s.client, workspaceID, key, expiry)
}

func (s *Service) PresignedPutURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	return s.presignedPutURL(ctx, s.client, workspaceID, key, expiry)
}

func (s *Service) PresignedSandboxGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	return s.presignedGetURL(ctx, s.sandboxClient, workspaceID, key, expiry)
}

func (s *Service) PresignedSandboxPutURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	return s.presignedPutURL(ctx, s.sandboxClient, workspaceID, key, expiry)
}

func (s *Service) presignedGetURL(ctx context.Context, client *minio.Client, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	key, err := CleanKey(key)
	if err != nil {
		return "", err
	}
	rawURL, err := client.PresignedGetObject(ctx, BucketName(workspaceID), key, expiry, nil)
	if err != nil {
		return "", err
	}
	return rawURL.String(), nil
}

func (s *Service) presignedPutURL(ctx context.Context, client *minio.Client, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	key, err := CleanKey(key)
	if err != nil {
		return "", err
	}
	rawURL, err := client.PresignedPutObject(ctx, BucketName(workspaceID), key, expiry)
	if err != nil {
		return "", err
	}
	return rawURL.String(), nil
}

func BucketName(workspaceID pgtype.UUID) string {
	return "workspace-" + uuidString(workspaceID)
}

func StorageURL(workspaceID pgtype.UUID, key string) string {
	return BucketName(workspaceID) + "/" + strings.TrimLeft(key, "/")
}

func (s *Service) StorageURL(workspaceID pgtype.UUID, key string) string {
	return StorageURL(workspaceID, key)
}

func CleanKey(key string) (string, error) {
	key = strings.TrimSpace(strings.TrimLeft(key, "/"))
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid object key")
	}
	return key, nil
}

func newClient(endpoint string, cfg config.MinIOConfig) (*minio.Client, error) {
	return minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: "us-east-1",
	})
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
