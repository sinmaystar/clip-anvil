package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/minio/minio-go/v7"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const maxUploadBytes = 100 << 20

type UploadHandler struct {
	queries *db.Queries
	minio   *minio.Client
}

type assetResponse struct {
	db.MediaAsset
	AccessURL string `json:"access_url"`
}

func NewUploadHandler(queries *db.Queries, minioClient *minio.Client) *UploadHandler {
	return &UploadHandler{queries: queries, minio: minioClient}
}

func (h *UploadHandler) Upload(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	workspaceID, ok := uuidFromString(c.PostForm("workspace_id"))
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !workspaceBelongsToAccount(ctx, h.queries, workspaceID, accountID, c) {
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if header.Size > maxUploadBytes {
		writeError(c, consts.StatusBadRequest, "file too large")
		return
	}
	file, err := header.Open()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	defer func() { _ = file.Close() }()

	mime, err := detectMultipartMIME(file)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	mediaType, ok := mediaTypeForMIME(mime)
	if !ok {
		writeError(c, consts.StatusBadRequest, "unsupported media type")
		return
	}

	bucketName := "workspace-" + uuidToString(workspaceID)
	if err := h.ensureBucket(ctx, bucketName); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to store asset")
		return
	}
	objectName := fmt.Sprintf("assets/%d/%s", time.Now().UnixNano(), safeFilename(header.Filename))
	if _, err := h.minio.PutObject(ctx, bucketName, objectName, file, header.Size, minio.PutObjectOptions{ContentType: mime}); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to store asset")
		return
	}

	asset, err := h.queries.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
		WorkspaceID: workspaceID,
		Type:        mediaType,
		Mime:        mime,
		StorageUrl:  bucketName + "/" + objectName,
		SizeBytes:   pgtype.Int8{Int64: header.Size, Valid: true},
		Metadata:    []byte("{}"),
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create asset")
		return
	}
	accessURL, err := h.presignedURL(ctx, bucketName, objectName)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create asset")
		return
	}

	c.JSON(consts.StatusOK, assetResponse{MediaAsset: asset, AccessURL: accessURL})
}

func mediaTypeForMIME(mime string) (db.MediaType, bool) {
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return db.MediaTypeImage, true
	case "video/mp4", "video/quicktime", "video/webm":
		return db.MediaTypeVideo, true
	case "audio/mpeg", "audio/wav", "audio/aac", "audio/ogg":
		return db.MediaTypeAudio, true
	default:
		return "", false
	}
}

func detectMultipartMIME(file multipart.File) (string, error) {
	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		return "", err
	}
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
	}
	return http.DetectContentType(head[:n]), nil
}

func (h *UploadHandler) ensureBucket(ctx context.Context, bucketName string) error {
	exists, err := h.minio.BucketExists(ctx, bucketName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return h.minio.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
}

func (h *UploadHandler) presignedURL(ctx context.Context, bucketName string, objectName string) (string, error) {
	url, err := h.minio.PresignedGetObject(ctx, bucketName, objectName, 15*time.Minute, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func safeFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "." || name == "" {
		return "upload.bin"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
}

func workspaceBelongsToAccount(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID, accountID pgtype.UUID, c *app.RequestContext) bool {
	workspace, err := queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return false
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return false
	}
	return true
}
