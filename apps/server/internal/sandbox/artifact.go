package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const MaxArtifactBytes int64 = 500 << 20

type ArtifactInput struct {
	Path      string
	Title     string
	NodeID    string
	MediaType string
}

type ArtifactResult struct {
	AssetID   string `json:"asset_id"`
	NodeID    string `json:"node_id"`
	AccessURL string `json:"access_url"`
}

type ArtifactObject struct {
	StorageURL string
	AccessURL  string
}

type ArtifactStorage interface {
	EnsureBucket(ctx context.Context, workspaceID pgtype.UUID) error
	PresignedSandboxPutURL(ctx context.Context, workspaceID pgtype.UUID, objectName string, expiry time.Duration) (string, error)
	PresignedGetURL(ctx context.Context, workspaceID pgtype.UUID, objectName string, expiry time.Duration) (string, error)
	StorageURL(workspaceID pgtype.UUID, objectName string) string
}

type ArtifactRepository interface {
	CreateMediaAsset(ctx context.Context, arg db.CreateMediaAssetParams) (db.MediaAsset, error)
	CreateAgentMediaNode(ctx context.Context, arg db.CreateAgentMediaNodeParams) (db.MediaNode, error)
	UpdateMediaNodeAsset(ctx context.Context, arg db.UpdateMediaNodeAssetParams) (db.MediaNode, error)
	UpdateMediaNodeStatus(ctx context.Context, arg db.UpdateMediaNodeStatusParams) (db.MediaNode, error)
}

type ArtifactBroadcaster interface {
	Broadcast(workspaceID pgtype.UUID, event string, payload map[string]any)
}

type ArtifactService struct {
	client      Client
	repository  ArtifactRepository
	storage     ArtifactStorage
	broadcaster ArtifactBroadcaster
}

func NewArtifactService(client Client, repository ArtifactRepository, storage ArtifactStorage, broadcaster ArtifactBroadcaster) *ArtifactService {
	return &ArtifactService{client: client, repository: repository, storage: storage, broadcaster: broadcaster}
}

func (s *ArtifactService) Submit(ctx context.Context, sandboxID string, workspaceID pgtype.UUID, input ArtifactInput) (ArtifactResult, error) {
	artifactPath, err := ValidateOutputPath(input.Path)
	if err != nil {
		return ArtifactResult{}, err
	}
	info, err := InspectArtifact(ctx, s.client, sandboxID, artifactPath)
	if err != nil {
		return ArtifactResult{}, err
	}
	if err := ValidateArtifactSize(info.SizeBytes); err != nil {
		return ArtifactResult{}, err
	}
	mediaType, ok := MediaTypeForArtifactMIME(info.Mime)
	if !ok {
		return ArtifactResult{}, fmt.Errorf("unsupported artifact MIME %q", info.Mime)
	}

	objectName := fmt.Sprintf("artifacts/%d/%s", time.Now().UnixNano(), SafeAssetName(path.Base(artifactPath)))
	if err := s.storage.EnsureBucket(ctx, workspaceID); err != nil {
		return ArtifactResult{}, err
	}
	putURL, err := s.storage.PresignedSandboxPutURL(ctx, workspaceID, objectName, time.Hour)
	if err != nil {
		return ArtifactResult{}, err
	}
	uploadResult, err := UploadToMinIO(ctx, s.client, sandboxID, artifactPath, putURL)
	if err != nil {
		return ArtifactResult{}, err
	}
	if uploadResult.ExitCode != 0 {
		return ArtifactResult{}, fmt.Errorf("artifact upload failed with exit code %d: %s", uploadResult.ExitCode, uploadResult.Stderr)
	}
	accessURL, err := s.storage.PresignedGetURL(ctx, workspaceID, objectName, 15*time.Minute)
	if err != nil {
		return ArtifactResult{}, err
	}
	asset, err := s.repository.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
		WorkspaceID: workspaceID,
		Type:        db.AssetType(mediaType),
		Mime:        info.Mime,
		StorageUrl:  pgtype.Text{String: s.storage.StorageURL(workspaceID, objectName), Valid: true},
		SizeBytes:   pgtype.Int8{Int64: info.SizeBytes, Valid: true},
		Metadata:    []byte("{}"),
	})
	if err != nil {
		return ArtifactResult{}, err
	}

	node, err := s.upsertNode(ctx, workspaceID, input, asset)
	if err != nil {
		return ArtifactResult{}, err
	}
	return ArtifactResult{
		AssetID:   uuidString(asset.ID),
		NodeID:    uuidString(node.ID),
		AccessURL: accessURL,
	}, nil
}

func InspectArtifact(ctx context.Context, client Client, sandboxID string, artifactPath string) (FileInfo, error) {
	command := "test -f " + shellQuote(artifactPath) +
		" && stat -c%s " + shellQuote(artifactPath) +
		" && file -b --mime-type " + shellQuote(artifactPath)
	result, err := RunExec(ctx, client, sandboxID, ExecInput{Command: command, TimeoutSeconds: 30})
	if err != nil {
		return FileInfo{}, err
	}
	if result.ExitCode != 0 {
		return FileInfo{}, fmt.Errorf("artifact inspect failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) < 2 {
		return FileInfo{}, errors.New("artifact inspect returned invalid output")
	}
	size, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Path:      artifactPath,
		SizeBytes: size,
		Mime:      strings.TrimSpace(lines[1]),
	}, nil
}

func DetectMIME(r io.Reader) (string, []byte, error) {
	head := make([]byte, 512)
	n, err := r.Read(head)
	if err != nil && err != io.EOF {
		return "", nil, err
	}
	return http.DetectContentType(head[:n]), head[:n], nil
}

func MediaTypeForArtifactMIME(mime string) (string, bool) {
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return "image", true
	case "video/mp4", "video/quicktime", "video/webm":
		return "video", true
	case "audio/mpeg", "audio/wav", "audio/aac", "audio/ogg":
		return "audio", true
	default:
		return "", false
	}
}

func ValidateArtifactSize(size int64) error {
	if size < 0 {
		return errors.New("artifact size is unknown")
	}
	if size > MaxArtifactBytes {
		return errors.New("artifact too large")
	}
	return nil
}

func (s *ArtifactService) upsertNode(ctx context.Context, workspaceID pgtype.UUID, input ArtifactInput, asset db.MediaAsset) (db.MediaNode, error) {
	if strings.TrimSpace(input.NodeID) != "" {
		nodeID, ok := parseUUID(input.NodeID)
		if !ok {
			return db.MediaNode{}, errors.New("invalid node id")
		}
		_, err := s.repository.UpdateMediaNodeAsset(ctx, db.UpdateMediaNodeAssetParams{
			ID:      nodeID,
			AssetID: asset.ID,
		})
		if err != nil {
			return db.MediaNode{}, err
		}
		node, err := s.repository.UpdateMediaNodeStatus(ctx, db.UpdateMediaNodeStatusParams{
			ID:     nodeID,
			Status: db.NodeStatusSucceeded,
		})
		if err != nil {
			return db.MediaNode{}, err
		}
		s.broadcast(workspaceID, "NodeUpdated", map[string]any{"node": node})
		return node, nil
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "Artifact"
	}
	node, err := s.repository.CreateAgentMediaNode(ctx, db.CreateAgentMediaNodeParams{
		WorkspaceID: workspaceID,
		NodeType:    nodeTypeForAssetType(asset.Type),
		Title:       title,
		Prompt:      "",
		AssetID:     asset.ID,
		CanvasX:     0,
		CanvasY:     0,
		CanvasW:     320,
		CanvasH:     180,
	})
	if err != nil {
		return db.MediaNode{}, err
	}
	s.broadcast(workspaceID, "NodeCreated", map[string]any{"node": node})
	return node, nil
}

func nodeTypeForAssetType(assetType db.AssetType) db.NodeType {
	switch assetType {
	case db.AssetTypeText:
		return db.NodeTypeText
	case db.AssetTypeImage:
		return db.NodeTypeImage
	case db.AssetTypeVideo:
		return db.NodeTypeVideo
	case db.AssetTypeAudio:
		return db.NodeTypeAudio
	default:
		return db.NodeType("")
	}
}

func (s *ArtifactService) broadcast(workspaceID pgtype.UUID, event string, payload map[string]any) {
	if s.broadcaster == nil {
		return
	}
	s.broadcaster.Broadcast(workspaceID, event, payload)
}

func parseUUID(input string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(input)); err != nil {
		return pgtype.UUID{}, false
	}
	return id, id.Valid
}
