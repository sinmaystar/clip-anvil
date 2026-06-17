package api

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type SandboxEnsurer interface {
	EnsureSandbox(ctx context.Context, workspaceID pgtype.UUID) (sandbox.WorkspaceSandbox, error)
	DeleteSandbox(ctx context.Context, workspaceID pgtype.UUID) (sandbox.WorkspaceSandbox, error)
}

type ArtifactSubmitter interface {
	Submit(ctx context.Context, sandboxID string, workspaceID pgtype.UUID, input sandbox.ArtifactInput) (sandbox.ArtifactResult, error)
}

type SandboxTransferStorage interface {
	EnsureBucket(ctx context.Context, workspaceID pgtype.UUID) error
	PresignedSandboxGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
	PresignedSandboxPutURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
}

type SandboxHandler struct {
	queries   *db.Queries
	manager   SandboxEnsurer
	client    sandbox.Client
	artifacts ArtifactSubmitter
	storage   SandboxTransferStorage
}

type sandboxStatusResponse struct {
	WorkspaceID string `json:"workspace_id"`
	SandboxID   string `json:"sandbox_id"`
	VolumeName  string `json:"volume_name"`
}

type sandboxExecRequest struct {
	Command        string `json:"command"`
	Cwd            string `json:"cwd"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type submitArtifactRequest struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	NodeID string `json:"node_id"`
}

type sandboxDownloadFromMinIORequest struct {
	Key      string `json:"key"`
	DestPath string `json:"dest_path"`
}

type sandboxUploadToMinIORequest struct {
	Key     string `json:"key"`
	SrcPath string `json:"src_path"`
}

func NewSandboxHandler(queries *db.Queries, manager SandboxEnsurer, client sandbox.Client, artifacts ArtifactSubmitter, storage SandboxTransferStorage) *SandboxHandler {
	return &SandboxHandler{queries: queries, manager: manager, client: client, artifacts: artifacts, storage: storage}
}

func (h *SandboxHandler) Status(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return
	}
	if !workspaceBelongsToAccount(ctx, h.queries, workspaceID, accountID, c) {
		return
	}
	info, err := h.manager.EnsureSandbox(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to ensure sandbox")
		return
	}
	c.JSON(consts.StatusOK, sandboxStatusResponse{
		WorkspaceID: uuidToString(info.WorkspaceID),
		SandboxID:   info.SandboxID,
		VolumeName:  info.VolumeName,
	})
}

func (h *SandboxHandler) Exec(ctx context.Context, c *app.RequestContext) {
	workspaceID, info, ok := h.ensureForRequest(ctx, c)
	if !ok {
		return
	}
	var req sandboxExecRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	result, err := sandbox.RunExec(ctx, h.client, info.SandboxID, sandbox.ExecInput{
		Command:        req.Command,
		Cwd:            req.Cwd,
		TimeoutSeconds: req.TimeoutSeconds,
	})
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	_ = workspaceID
	c.JSON(consts.StatusOK, result)
}

func (h *SandboxHandler) SubmitArtifact(ctx context.Context, c *app.RequestContext) {
	workspaceID, info, ok := h.ensureForRequest(ctx, c)
	if !ok {
		return
	}
	var req submitArtifactRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	result, err := h.artifacts.Submit(ctx, info.SandboxID, workspaceID, sandbox.ArtifactInput{
		Path:   req.Path,
		Title:  req.Title,
		NodeID: req.NodeID,
	})
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	c.JSON(consts.StatusOK, result)
}

func (h *SandboxHandler) Delete(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return
	}
	if !workspaceBelongsToAccount(ctx, h.queries, workspaceID, accountID, c) {
		return
	}
	info, err := h.manager.DeleteSandbox(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to delete sandbox")
		return
	}
	c.JSON(consts.StatusOK, sandboxStatusResponse{
		WorkspaceID: uuidToString(info.WorkspaceID),
		SandboxID:   info.SandboxID,
		VolumeName:  info.VolumeName,
	})
}

func (h *SandboxHandler) DownloadFromMinIO(ctx context.Context, c *app.RequestContext) {
	workspaceID, info, ok := h.ensureForRequest(ctx, c)
	if !ok {
		return
	}
	var req sandboxDownloadFromMinIORequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	getURL, err := h.storage.PresignedSandboxGetURL(ctx, workspaceID, req.Key, time.Hour)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "failed to prepare download")
		return
	}
	result, err := sandbox.DownloadFromMinIO(ctx, h.client, info.SandboxID, getURL, req.DestPath)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	c.JSON(consts.StatusOK, result)
}

func (h *SandboxHandler) UploadToMinIO(ctx context.Context, c *app.RequestContext) {
	workspaceID, info, ok := h.ensureForRequest(ctx, c)
	if !ok {
		return
	}
	var req sandboxUploadToMinIORequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if err := h.storage.EnsureBucket(ctx, workspaceID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to prepare upload")
		return
	}
	putURL, err := h.storage.PresignedSandboxPutURL(ctx, workspaceID, req.Key, time.Hour)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "failed to prepare upload")
		return
	}
	result, err := sandbox.UploadToMinIO(ctx, h.client, info.SandboxID, req.SrcPath, putURL)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	c.JSON(consts.StatusOK, result)
}

func (h *SandboxHandler) ensureForRequest(ctx context.Context, c *app.RequestContext) (pgtype.UUID, sandbox.WorkspaceSandbox, bool) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return pgtype.UUID{}, sandbox.WorkspaceSandbox{}, false
	}
	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return pgtype.UUID{}, sandbox.WorkspaceSandbox{}, false
	}
	if !workspaceBelongsToAccount(ctx, h.queries, workspaceID, accountID, c) {
		return pgtype.UUID{}, sandbox.WorkspaceSandbox{}, false
	}
	info, err := h.manager.EnsureSandbox(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to ensure sandbox")
		return pgtype.UUID{}, sandbox.WorkspaceSandbox{}, false
	}
	return workspaceID, info, true
}
