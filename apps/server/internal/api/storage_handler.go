package api

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type StorageHandler struct {
	queries *db.Queries
	storage *storage.Service
}

type storageUploadResponse struct {
	Key        string `json:"key"`
	StorageURL string `json:"storage_url"`
}

type presignedUploadRequest struct {
	Key string `json:"key"`
}

type presignedUploadResponse struct {
	Key        string `json:"key"`
	StorageURL string `json:"storage_url"`
	UploadURL  string `json:"upload_url"`
}

type completeUploadRequest struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

func NewStorageHandler(queries *db.Queries, storageService *storage.Service) *StorageHandler {
	return &StorageHandler{queries: queries, storage: storageService}
}

func (h *StorageHandler) Upload(ctx context.Context, c *app.RequestContext) {
	workspaceID, ok := h.workspaceForRequest(ctx, c)
	if !ok {
		return
	}
	key := c.PostForm("key")
	if key == "" {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
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
	object, err := h.storage.Upload(ctx, workspaceID, key, file, header.Size, mime)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to store object")
		return
	}
	c.JSON(consts.StatusOK, storageUploadResponse{Key: object.Key, StorageURL: object.StorageURL})
}

func (h *StorageHandler) PresignedUpload(ctx context.Context, c *app.RequestContext) {
	workspaceID, ok := h.workspaceForRequest(ctx, c)
	if !ok {
		return
	}
	var req presignedUploadRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	key, err := storage.CleanKey(req.Key)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if err := h.storage.EnsureBucket(ctx, workspaceID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to prepare upload")
		return
	}
	uploadURL, err := h.storage.PresignedPutURL(ctx, workspaceID, key, time.Hour)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to prepare upload")
		return
	}
	c.JSON(consts.StatusOK, presignedUploadResponse{
		Key:        key,
		StorageURL: h.storage.StorageURL(workspaceID, key),
		UploadURL:  uploadURL,
	})
}

func (h *StorageHandler) CompleteUpload(ctx context.Context, c *app.RequestContext) {
	workspaceID, ok := h.workspaceForRequest(ctx, c)
	if !ok {
		return
	}
	var req completeUploadRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	object, err := h.storage.StatObject(ctx, workspaceID, req.Key)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "object not found")
		return
	}
	mediaType, ok := mediaTypeForMIME(object.MIME)
	if !ok {
		writeError(c, consts.StatusBadRequest, fmt.Sprintf("unsupported media type %q", object.MIME))
		return
	}
	asset, err := h.queries.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
		WorkspaceID: workspaceID,
		Type:        mediaType,
		Mime:        object.MIME,
		StorageUrl:  object.StorageURL,
		SizeBytes:   pgtype.Int8{Int64: object.Size, Valid: true},
		Metadata:    []byte("{}"),
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create asset")
		return
	}
	accessURL, err := h.storage.PresignedGetURL(ctx, workspaceID, object.Key, 15*time.Minute)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create asset")
		return
	}
	_ = req.Title
	c.JSON(consts.StatusOK, assetResponse{MediaAsset: asset, AccessURL: accessURL})
}

func (h *StorageHandler) workspaceForRequest(ctx context.Context, c *app.RequestContext) (pgtype.UUID, bool) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return pgtype.UUID{}, false
	}
	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return pgtype.UUID{}, false
	}
	if !workspaceBelongsToAccount(ctx, h.queries, workspaceID, accountID, c) {
		return pgtype.UUID{}, false
	}
	return workspaceID, true
}
