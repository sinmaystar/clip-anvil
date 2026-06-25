package api

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type CanvasHandler struct {
	queries *db.Queries
	storage assetURLSigner
}

func NewCanvasHandler(queries *db.Queries, storage ...assetURLSigner) *CanvasHandler {
	handler := &CanvasHandler{queries: queries}
	if len(storage) > 0 {
		handler.storage = storage[0]
	}
	return handler
}

type cameraResponse struct {
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Zoom float32 `json:"zoom"`
}

type canvasResponse struct {
	Camera           cameraResponse                 `json:"camera"`
	Nodes            []canvasNodeResponse           `json:"nodes"`
	Edges            []mediaEdgeResponse            `json:"edges"`
	Groups           []canvasGroupResponse          `json:"groups"`
	DomainProjection domainCanvasProjectionResponse `json:"domain_projection"`
}

type canvasNodeResponse struct {
	mediaNodeResponse
	ThumbnailURL           *string                     `json:"thumbnail_url,omitempty"`
	ProductionPreview      *canvasProductionPreview    `json:"production_preview,omitempty"`
	ReferencePackPreview   *canvasReferencePackPreview `json:"reference_pack_preview,omitempty"`
	ActiveStaleReasonCount int                         `json:"active_stale_reason_count"`
}

type canvasProductionPreview struct {
	VersionID    string `json:"version_id"`
	VersionNo    int32  `json:"version_no"`
	AssetID      string `json:"asset_id,omitempty"`
	AssetType    string `json:"asset_type,omitempty"`
	Mime         string `json:"mime,omitempty"`
	AccessURL    string `json:"access_url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Text         string `json:"text,omitempty"`
	Width        int32  `json:"width,omitempty"`
	Height       int32  `json:"height,omitempty"`
	DurationMS   int32  `json:"duration_ms,omitempty"`
	InputHash    string `json:"input_hash,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type canvasReferencePackPreview struct {
	MemberCount int                                `json:"member_count"`
	Members     []canvasReferencePackPreviewMember `json:"members"`
}

type canvasReferencePackPreviewMember struct {
	ID            string `json:"id"`
	NodeType      string `json:"node_type"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	OperationType string `json:"operation_type,omitempty"`
	AssetID       string `json:"asset_id,omitempty"`
}

type canvasGroupResponse struct {
	ID          pgtype.UUID   `json:"id"`
	WorkspaceID pgtype.UUID   `json:"workspace_id"`
	Name        string        `json:"name"`
	SortOrder   int32         `json:"sort_order"`
	NodeIDs     []pgtype.UUID `json:"node_ids"`
}

type updateCameraRequest struct {
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Zoom float32 `json:"zoom"`
}

func (h *CanvasHandler) GetCanvas(ctx context.Context, c *app.RequestContext) {
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

	canvas, err := h.queries.GetCanvasDocumentByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load canvas")
		return
	}
	nodes, err := h.queries.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load nodes")
		return
	}
	edges, err := h.queries.ListMediaEdgesByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load edges")
		return
	}
	groups, err := h.queries.ListMediaGroupsByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load groups")
		return
	}
	assets, err := h.queries.ListMediaAssetsByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load assets")
		return
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID, err := h.currentVersionsByID(ctx, nodes)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load current versions")
		return
	}
	staleCounts, err := h.activeStaleReasonCountsByNode(ctx, nodes)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load stale reasons")
		return
	}
	packMembers, err := h.referencePackMembersByPack(ctx, nodes)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load reference pack members")
		return
	}

	nodeResponses, err := toCanvasNodeResponsesWithSigner(ctx, h.storage, nodes, assetsByID, versionsByID, staleCounts, packMembers)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to sign production preview")
		return
	}
	domainProjection, err := buildDomainCanvasProjection(ctx, h.queries, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load domain projection")
		return
	}

	c.JSON(consts.StatusOK, canvasResponse{
		Camera:           toCameraResponse(canvas),
		Nodes:            nodeResponses,
		Edges:            toMediaEdgeResponses(edges),
		Groups:           toCanvasGroupResponses(groups, nodes),
		DomainProjection: domainProjection,
	})
}

func (h *CanvasHandler) UpdateCamera(ctx context.Context, c *app.RequestContext) {
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

	if _, ok := requireCanvasLayoutWorkspace(ctx, h.queries, workspaceID, accountID, c); !ok {
		return
	}

	var req updateCameraRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if req.Zoom <= 0 {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	if _, err := h.queries.UpdateCamera(ctx, db.UpdateCameraParams{
		WorkspaceID: workspaceID,
		CameraX:     req.X,
		CameraY:     req.Y,
		CameraZoom:  req.Zoom,
	}); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update camera")
		return
	}

	c.Status(consts.StatusNoContent)
}

func toCameraResponse(canvas db.CanvasDocument) cameraResponse {
	return cameraResponse{
		X:    canvas.CameraX,
		Y:    canvas.CameraY,
		Zoom: canvas.CameraZoom,
	}
}

func (h *CanvasHandler) currentVersionsByID(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID]db.ArtifactVersion, error) {
	out := map[pgtype.UUID]db.ArtifactVersion{}
	for _, node := range nodes {
		if !node.CurrentVersionID.Valid {
			continue
		}
		version, err := h.queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return nil, err
		}
		out[node.CurrentVersionID] = version
	}
	return out, nil
}

func (h *CanvasHandler) activeStaleReasonCountsByNode(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID]int, error) {
	out := map[pgtype.UUID]int{}
	for _, node := range nodes {
		reasons, err := h.queries.ListActiveStaleReasonsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out[node.ID] = len(reasons)
	}
	return out, nil
}

func (h *CanvasHandler) referencePackMembersByPack(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID][]db.MediaNode, error) {
	out := map[pgtype.UUID][]db.MediaNode{}
	for _, node := range nodes {
		if node.NodeType != db.NodeTypeReferencePack {
			continue
		}
		members, err := h.queries.ListReferencePackItemNodes(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out[node.ID] = members
	}
	return out, nil
}

func toCanvasNodeResponses(
	nodes []db.MediaNode,
	assets map[pgtype.UUID]db.MediaAsset,
	versions map[pgtype.UUID]db.ArtifactVersion,
	staleCounts map[pgtype.UUID]int,
	packMembers map[pgtype.UUID][]db.MediaNode,
) []canvasNodeResponse {
	responses, _ := toCanvasNodeResponsesWithSigner(context.Background(), nil, nodes, assets, versions, staleCounts, packMembers)
	return responses
}

func toCanvasNodeResponsesWithSigner(
	ctx context.Context,
	signer assetURLSigner,
	nodes []db.MediaNode,
	assets map[pgtype.UUID]db.MediaAsset,
	versions map[pgtype.UUID]db.ArtifactVersion,
	staleCounts map[pgtype.UUID]int,
	packMembers map[pgtype.UUID][]db.MediaNode,
) ([]canvasNodeResponse, error) {
	responses := make([]canvasNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		response := canvasNodeResponse{
			mediaNodeResponse:      toMediaNodeResponse(node),
			ActiveStaleReasonCount: staleCounts[node.ID],
		}
		if node.AssetID.Valid {
			if asset, ok := assets[node.AssetID]; ok && asset.ThumbnailUrl.Valid {
				response.ThumbnailURL = &asset.ThumbnailUrl.String
			}
		}
		if node.CurrentVersionID.Valid {
			if version, ok := versions[node.CurrentVersionID]; ok {
				preview, err := toCanvasProductionPreview(ctx, signer, version, assets)
				if err != nil {
					return nil, err
				}
				response.ProductionPreview = preview
			}
		} else if node.AssetID.Valid {
			if asset, ok := assets[node.AssetID]; ok {
				preview, err := toCanvasDirectAssetPreview(ctx, signer, node, asset)
				if err != nil {
					return nil, err
				}
				response.ProductionPreview = preview
			}
		}
		if node.NodeType == db.NodeTypeReferencePack {
			response.ReferencePackPreview = toCanvasReferencePackPreview(packMembers[node.ID])
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func toCanvasProductionPreview(ctx context.Context, signer assetURLSigner, version db.ArtifactVersion, assets map[pgtype.UUID]db.MediaAsset) (*canvasProductionPreview, error) {
	preview := &canvasProductionPreview{
		VersionID: uuidToString(version.ID),
		VersionNo: version.VersionNo,
		InputHash: version.InputHash,
		CreatedAt: timeString(version.CreatedAt),
	}
	if version.AssetID.Valid {
		preview.AssetID = uuidToString(version.AssetID)
		if asset, ok := assets[version.AssetID]; ok {
			preview.AssetType = string(asset.Type)
			preview.Mime = asset.Mime
			accessURL, err := previewAssetAccessURL(ctx, signer, asset)
			if err != nil {
				return nil, err
			}
			preview.AccessURL = accessURL
			thumbnailURL, err := previewAssetThumbnailURL(ctx, signer, asset)
			if err != nil {
				return nil, err
			}
			preview.ThumbnailURL = thumbnailURL
			preview.Text = textString(asset.TextContent)
			applyAssetPreviewMetadata(preview, asset)
		}
	}
	return preview, nil
}

func toCanvasDirectAssetPreview(ctx context.Context, signer assetURLSigner, node db.MediaNode, asset db.MediaAsset) (*canvasProductionPreview, error) {
	preview := &canvasProductionPreview{
		VersionID: uuidToString(node.ID),
		VersionNo: 1,
		AssetID:   uuidToString(asset.ID),
		AssetType: string(asset.Type),
		Mime:      asset.Mime,
		Text:      textString(asset.TextContent),
		CreatedAt: timeString(node.CreatedAt),
	}
	accessURL, err := previewAssetAccessURL(ctx, signer, asset)
	if err != nil {
		return nil, err
	}
	preview.AccessURL = accessURL
	thumbnailURL, err := previewAssetThumbnailURL(ctx, signer, asset)
	if err != nil {
		return nil, err
	}
	preview.ThumbnailURL = thumbnailURL
	applyAssetPreviewMetadata(preview, asset)
	return preview, nil
}

func applyAssetPreviewMetadata(preview *canvasProductionPreview, asset db.MediaAsset) {
	if asset.DurationMs.Valid {
		preview.DurationMS = asset.DurationMs.Int32
	}
	width, height := dimensionsFromMetadata(asset.Metadata)
	preview.Width = width
	preview.Height = height
}

func dimensionsFromMetadata(raw []byte) (int32, int32) {
	if len(raw) == 0 {
		return 0, 0
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return 0, 0
	}
	return int32FromMetadata(metadata["width"]), int32FromMetadata(metadata["height"])
}

func int32FromMetadata(value any) int32 {
	switch typed := value.(type) {
	case float64:
		if typed > 0 && typed <= float64(math.MaxInt32) {
			return int32(typed)
		}
	case int:
		if typed > 0 {
			return int32(typed)
		}
	}
	return 0
}

func previewAssetThumbnailURL(ctx context.Context, signer assetURLSigner, asset db.MediaAsset) (string, error) {
	if !asset.ThumbnailUrl.Valid || strings.TrimSpace(asset.ThumbnailUrl.String) == "" {
		return "", nil
	}
	if strings.HasPrefix(asset.ThumbnailUrl.String, "http://") || strings.HasPrefix(asset.ThumbnailUrl.String, "https://") {
		return asset.ThumbnailUrl.String, nil
	}
	if signer == nil {
		return asset.ThumbnailUrl.String, nil
	}
	key, err := storage.KeyFromStorageURL(asset.WorkspaceID, asset.ThumbnailUrl.String)
	if err != nil {
		return "", err
	}
	return signer.PresignedGetURL(ctx, asset.WorkspaceID, key, 15*time.Minute)
}

func previewAssetAccessURL(ctx context.Context, signer assetURLSigner, asset db.MediaAsset) (string, error) {
	if !asset.StorageUrl.Valid {
		return "", nil
	}
	if signer == nil {
		return asset.StorageUrl.String, nil
	}
	key, err := storage.KeyFromStorageURL(asset.WorkspaceID, asset.StorageUrl.String)
	if err != nil {
		return "", err
	}
	return signer.PresignedGetURL(ctx, asset.WorkspaceID, key, 15*time.Minute)
}

func toCanvasReferencePackPreview(members []db.MediaNode) *canvasReferencePackPreview {
	preview := &canvasReferencePackPreview{
		MemberCount: len(members),
		Members:     make([]canvasReferencePackPreviewMember, 0, previewMemberCapacity(len(members))),
	}
	for i, member := range members {
		if i >= 3 {
			break
		}
		preview.Members = append(preview.Members, canvasReferencePackPreviewMember{
			ID:            uuidToString(member.ID),
			NodeType:      string(member.NodeType),
			Title:         member.Title,
			Status:        string(member.Status),
			OperationType: member.OperationType,
			AssetID:       uuidStringForCanvas(member.AssetID),
		})
	}
	return preview
}

func uuidStringForCanvas(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuidToString(value)
}

func previewMemberCapacity(count int) int {
	if count > 3 {
		return 3
	}
	return count
}

func toCanvasGroupResponses(groups []db.MediaGroup, nodes []db.MediaNode) []canvasGroupResponse {
	nodeIDsByGroup := make(map[pgtype.UUID][]pgtype.UUID, len(groups))
	for _, node := range nodes {
		if node.GroupID.Valid {
			nodeIDsByGroup[node.GroupID] = append(nodeIDsByGroup[node.GroupID], node.ID)
		}
	}

	responses := make([]canvasGroupResponse, 0, len(groups))
	for _, group := range groups {
		responses = append(responses, canvasGroupResponse{
			ID:          group.ID,
			WorkspaceID: group.WorkspaceID,
			Name:        group.Name,
			SortOrder:   group.SortOrder,
			NodeIDs:     nodeIDsByGroup[group.ID],
		})
	}
	return responses
}
