package craftsman

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ContextStore interface {
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	ListMediaNodesByShot(ctx context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error)
	ListGenerationJobsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error)
	ListArtifactVersionsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error)
}

type MessageRuntime interface {
	ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error)
}

type ContextLoader struct {
	Store   ContextStore
	Runtime MessageRuntime
}

func (l ContextLoader) Load(ctx context.Context, input GraphInput) (Context, error) {
	if l.Store == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid || !input.ShotID.Valid {
		return Context{}, ErrInvalidInput
	}
	shot, err := l.Store.GetShotByID(ctx, input.ShotID)
	if err != nil {
		return Context{}, err
	}
	if shot.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	messages := []db.AgentMessage{}
	if l.Runtime != nil {
		messages, err = l.Runtime.ListMessages(ctx, input.ThreadID, 0, 50)
		if err != nil {
			return Context{}, err
		}
	}
	nodes, err := l.Store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{
		WorkspaceID: input.WorkspaceID,
		ShotID:      input.ShotID,
	})
	if err != nil {
		return Context{}, err
	}
	nodeStates := make([]NodeState, 0, len(nodes))
	for _, node := range nodes {
		jobs, err := l.Store.ListGenerationJobsByNode(ctx, node.ID)
		if err != nil {
			return Context{}, err
		}
		versions, err := l.Store.ListArtifactVersionsByNode(ctx, node.ID)
		if err != nil {
			return Context{}, err
		}
		nodeStates = append(nodeStates, NodeState{Node: node, Jobs: jobs, Versions: versions})
	}
	structured := map[string]any{
		"shot": map[string]any{
			"id":         uuidString(shot.ID),
			"client_key": shot.ClientKey,
			"title":      shot.Title,
			"status":     shot.Status,
		},
		"node_count": len(nodeStates),
	}
	return Context{
		Input:      input,
		Shot:       shot,
		Messages:   messages,
		Nodes:      nodeStates,
		Text:       buildContextText(shot, nodeStates),
		Structured: structured,
	}, nil
}

func buildContextText(shot db.Shot, nodes []NodeState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Shot\n")
	fmt.Fprintf(&b, "- id: %s\n", uuidString(shot.ID))
	fmt.Fprintf(&b, "- client_key: %s\n", shot.ClientKey)
	fmt.Fprintf(&b, "- title: %s\n", shot.Title)
	fmt.Fprintf(&b, "- status: %s\n", shot.Status)
	if len(nodes) == 0 {
		fmt.Fprintf(&b, "\nNodes\n- none\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\nNodes\n")
	for _, node := range nodes {
		fmt.Fprintf(&b, "- %s (%s, %s)", node.Node.Title, node.Node.NodeType, node.Node.Status)
		if len(node.Jobs) > 0 {
			latest := node.Jobs[len(node.Jobs)-1]
			fmt.Fprintf(&b, " latest_job=%s/%s", latest.OperationType, latest.Status)
		}
		if len(node.Versions) > 0 {
			latest := node.Versions[len(node.Versions)-1]
			fmt.Fprintf(&b, " latest_version=%s", latest.Status)
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
