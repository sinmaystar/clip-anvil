package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type inputRefStore interface {
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	GetDependencyEdgeByEndpoints(ctx context.Context, params db.GetDependencyEdgeByEndpointsParams) (db.MediaEdge, error)
	CreateMediaEdge(ctx context.Context, params db.CreateMediaEdgeParams) (db.MediaEdge, error)
}

func ResolveInputRefs(ctx context.Context, store inputRefStore, workspaceID pgtype.UUID, targetNodeID pgtype.UUID, values []string) ([]production.InputRef, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if store == nil || !workspaceID.Valid || !targetNodeID.Valid {
		return nil, ErrInvalidConfig
	}
	nodes, err := store.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	refs := make([]production.InputRef, 0, len(values))
	for _, value := range values {
		node, err := resolveInputNode(nodes, value)
		if err != nil {
			return nil, err
		}
		ref, err := inputRefForNode(ctx, store, node)
		if err != nil {
			return nil, err
		}
		ref.Kind = production.InputKindExplicit
		ref.Required = true
		if err := ensureDependencyEdge(ctx, store, workspaceID, node.ID, targetNodeID); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func resolveInputNode(nodes []db.MediaNode, value string) (db.MediaNode, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return db.MediaNode{}, fmt.Errorf("%w: empty input node ref", ErrInvalidInput)
	}
	if id, ok := pgUUIDFromString(value); ok {
		for _, node := range nodes {
			if node.ID == id {
				return node, nil
			}
		}
		return db.MediaNode{}, fmt.Errorf("%w: input node %s not found", ErrInvalidInput, value)
	}
	if node, ok := resolveInputNodeBySemanticKey(nodes, value); ok {
		return node, nil
	}
	if node, ok, err := resolveCurrentShotOutputNode(nodes, value); ok || err != nil {
		if err != nil {
			return db.MediaNode{}, err
		}
		return node, nil
	}
	var matches []db.MediaNode
	for _, node := range nodes {
		if strings.EqualFold(strings.TrimSpace(node.Title), value) {
			matches = append(matches, node)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return db.MediaNode{}, fmt.Errorf("%w: input node %q not found", ErrInvalidInput, value)
	default:
		return db.MediaNode{}, fmt.Errorf("%w: input node title %q is ambiguous", ErrInvalidInput, value)
	}
}

func resolveInputNodeBySemanticKey(nodes []db.MediaNode, value string) (db.MediaNode, bool) {
	for _, node := range nodes {
		if strings.TrimSpace(node.SemanticKey) == value {
			return node, true
		}
	}
	return db.MediaNode{}, false
}

func resolveCurrentShotOutputNode(nodes []db.MediaNode, value string) (db.MediaNode, bool, error) {
	const suffix = ".current"
	if !strings.HasSuffix(value, suffix) {
		return db.MediaNode{}, false, nil
	}
	base := strings.TrimSuffix(value, suffix)
	dot := strings.LastIndex(base, ".")
	if dot <= 0 || dot >= len(base)-1 {
		return db.MediaNode{}, false, fmt.Errorf("%w: input selector %q is invalid", ErrInvalidInput, value)
	}
	artifactKind := base[dot+1:]
	var matches []db.MediaNode
	for _, node := range nodes {
		if strings.TrimSpace(node.ArtifactKind) != artifactKind || !node.CurrentVersionID.Valid {
			continue
		}
		semanticKey := strings.TrimSpace(node.SemanticKey)
		if semanticKey == "" {
			continue
		}
		if strings.HasPrefix(semanticKey, base+".") || strings.Contains(semanticKey, "."+base+".") {
			matches = append(matches, node)
		}
	}
	switch len(matches) {
	case 0:
		return db.MediaNode{}, false, fmt.Errorf("%w: input selector %q not found", ErrInvalidInput, value)
	case 1:
		return matches[0], true, nil
	default:
		return latestMediaNode(matches), true, nil
	}
}

func latestMediaNode(nodes []db.MediaNode) db.MediaNode {
	latest := nodes[0]
	for _, node := range nodes[1:] {
		if node.UpdatedAt.Valid && (!latest.UpdatedAt.Valid || node.UpdatedAt.Time.After(latest.UpdatedAt.Time)) {
			latest = node
		}
	}
	return latest
}

func inputRefForNode(ctx context.Context, store inputRefStore, node db.MediaNode) (production.InputRef, error) {
	ref := production.InputRef{
		NodeID:   node.ID,
		NodeType: string(node.NodeType),
	}
	if node.CurrentVersionID.Valid {
		version, err := store.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return production.InputRef{}, err
		}
		ref.CurrentVersionID = uuidString(version.ID)
		ref.InputHash = version.InputHash
		if version.AssetID.Valid {
			return inputRefWithAsset(ctx, store, ref, version.AssetID)
		}
		return ref, nil
	}
	if node.NodeType == db.NodeTypeText && node.Prompt != "" && !node.AssetID.Valid {
		ref.AssetType = string(db.AssetTypeText)
		ref.TextContent = node.Prompt
		return ref, nil
	}
	if node.AssetID.Valid {
		return inputRefWithAsset(ctx, store, ref, node.AssetID)
	}
	return ref, nil
}

func inputRefWithAsset(ctx context.Context, store inputRefStore, ref production.InputRef, assetID pgtype.UUID) (production.InputRef, error) {
	asset, err := store.GetMediaAssetByID(ctx, assetID)
	if err != nil {
		return production.InputRef{}, err
	}
	ref.AssetID = uuidString(asset.ID)
	ref.AssetType = string(asset.Type)
	ref.Mime = asset.Mime
	ref.StorageURL = nullableTextString(asset.StorageUrl)
	ref.TextContent = nullableTextString(asset.TextContent)
	return ref, nil
}

func ensureDependencyEdge(ctx context.Context, store inputRefStore, workspaceID pgtype.UUID, fromNodeID pgtype.UUID, toNodeID pgtype.UUID) error {
	if fromNodeID == toNodeID {
		return nil
	}
	_, err := store.GetDependencyEdgeByEndpoints(ctx, db.GetDependencyEdgeByEndpointsParams{
		FromNodeID: fromNodeID,
		ToNodeID:   toNodeID,
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = store.CreateMediaEdge(ctx, db.CreateMediaEdgeParams{
		WorkspaceID: workspaceID,
		FromNodeID:  fromNodeID,
		ToNodeID:    toNodeID,
		Metadata:    []byte(`{}`),
	})
	return err
}

func nullableTextString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
