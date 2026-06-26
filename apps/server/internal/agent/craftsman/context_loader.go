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
	GetKeyElementStateByID(ctx context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error)
	ListMediaNodesByShot(ctx context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error)
	ListSourceMaterialNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	ListShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error)
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

const craftsmanContextMessageLimit int32 = 1000

func (l ContextLoader) Load(ctx context.Context, input GraphInput) (Context, error) {
	if input.ScopeType == "" {
		input.ScopeType = "shot"
	}
	if !input.ScopeID.Valid && input.ShotID.Valid {
		input.ScopeID = input.ShotID
	}
	if l.Store == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid || !input.ScopeID.Valid {
		return Context{}, ErrInvalidInput
	}
	messages := []db.AgentMessage{}
	var err error
	if l.Runtime != nil {
		messages, err = l.Runtime.ListMessages(ctx, input.ThreadID, 0, craftsmanContextMessageLimit)
		if err != nil {
			return Context{}, err
		}
	}
	if input.ScopeType == "key_element_state" {
		return l.loadKeyElementStateContext(ctx, input, messages)
	}
	return l.loadShotContext(ctx, input, messages)
}

func (l ContextLoader) loadShotContext(ctx context.Context, input GraphInput, messages []db.AgentMessage) (Context, error) {
	shotID := input.ShotID
	if !shotID.Valid {
		shotID = input.ScopeID
	}
	shot, err := l.Store.GetShotByID(ctx, shotID)
	if err != nil {
		return Context{}, err
	}
	if shot.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	nodes, err := l.Store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{
		WorkspaceID: input.WorkspaceID,
		ShotID:      shotID,
	})
	if err != nil {
		return Context{}, err
	}
	nodeStates := make([]NodeState, 0, len(nodes))
	for _, node := range nodes {
		state, err := l.loadNodeState(ctx, node)
		if err != nil {
			return Context{}, err
		}
		nodeStates = append(nodeStates, state)
	}
	dependencies, err := l.Store.ListShotDependenciesByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return Context{}, err
	}
	dependencies = filterShotDependencies(dependencies, shotID)
	sourceNodes, err := l.Store.ListSourceMaterialNodesByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return Context{}, err
	}
	sourceMaterials := make([]NodeState, 0, len(sourceNodes))
	for _, node := range sourceNodes {
		state, err := l.loadNodeState(ctx, node)
		if err != nil {
			return Context{}, err
		}
		sourceMaterials = append(sourceMaterials, state)
	}
	structured := map[string]any{
		"task": map[string]any{
			"target_phase":     input.Mode,
			"execution_policy": input.ExecutionPolicy,
		},
		"scope": map[string]any{
			"type": "shot",
			"id":   uuidString(shotID),
		},
		"shot": map[string]any{
			"id":         uuidString(shot.ID),
			"client_key": shot.ClientKey,
			"title":      shot.Title,
			"status":     shot.Status,
		},
		"node_count":            len(nodeStates),
		"dependency_count":      len(dependencies),
		"source_material_count": len(sourceMaterials),
	}
	return Context{
		Input:           input,
		Shot:            shot,
		Messages:        messages,
		Nodes:           nodeStates,
		Dependencies:    dependencies,
		SourceMaterials: sourceMaterials,
		Text:            buildContextText(input, shot, nodeStates, dependencies, sourceMaterials),
		Structured:      structured,
	}, nil
}

func (l ContextLoader) loadKeyElementStateContext(ctx context.Context, input GraphInput, messages []db.AgentMessage) (Context, error) {
	state, err := l.Store.GetKeyElementStateByID(ctx, db.GetKeyElementStateByIDParams{ID: input.ScopeID, WorkspaceID: input.WorkspaceID})
	if err != nil {
		return Context{}, err
	}
	if state.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	sourceNodes, err := l.Store.ListSourceMaterialNodesByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return Context{}, err
	}
	sourceMaterials := make([]NodeState, 0, len(sourceNodes))
	for _, node := range sourceNodes {
		nodeState, err := l.loadNodeState(ctx, node)
		if err != nil {
			return Context{}, err
		}
		sourceMaterials = append(sourceMaterials, nodeState)
	}
	structured := map[string]any{
		"task": map[string]any{
			"target_phase":     input.Mode,
			"execution_policy": input.ExecutionPolicy,
		},
		"scope": map[string]any{
			"type": "key_element_state",
			"id":   uuidString(state.ID),
		},
		"key_element_state": map[string]any{
			"id":               uuidString(state.ID),
			"client_key":       state.ClientKey,
			"label":            state.Label,
			"reference_status": state.ReferenceStatus,
		},
		"source_material_count": len(sourceMaterials),
	}
	return Context{
		Input:           input,
		KeyElementState: state,
		Messages:        messages,
		SourceMaterials: sourceMaterials,
		Text:            buildKeyElementStateContextText(input, state, sourceMaterials),
		Structured:      structured,
	}, nil
}

func (l ContextLoader) loadNodeState(ctx context.Context, node db.MediaNode) (NodeState, error) {
	jobs, err := l.Store.ListGenerationJobsByNode(ctx, node.ID)
	if err != nil {
		return NodeState{}, err
	}
	versions, err := l.Store.ListArtifactVersionsByNode(ctx, node.ID)
	if err != nil {
		return NodeState{}, err
	}
	return NodeState{Node: node, Jobs: jobs, Versions: versions}, nil
}

func filterShotDependencies(dependencies []db.ShotDependency, shotID pgtype.UUID) []db.ShotDependency {
	out := make([]db.ShotDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		if dependency.FromShotID == shotID || dependency.ToShotID == shotID {
			out = append(out, dependency)
		}
	}
	return out
}

func buildContextText(input GraphInput, shot db.Shot, nodes []NodeState, dependencies []db.ShotDependency, sourceMaterials []NodeState) string {
	var b strings.Builder
	writeTaskContext(&b, input)
	fmt.Fprintf(&b, "Shot\n")
	fmt.Fprintf(&b, "- id: %s\n", uuidString(shot.ID))
	fmt.Fprintf(&b, "- client_key: %s\n", shot.ClientKey)
	fmt.Fprintf(&b, "- title: %s\n", shot.Title)
	fmt.Fprintf(&b, "- status: %s\n", shot.Status)
	if len(nodes) == 0 {
		fmt.Fprintf(&b, "\nNodes\n- none\n")
	} else {
		fmt.Fprintf(&b, "\nNodes\n")
		for _, node := range nodes {
			writeNodeState(&b, node)
		}
	}
	if len(dependencies) == 0 {
		fmt.Fprintf(&b, "\nDependencies\n- none\n")
	} else {
		fmt.Fprintf(&b, "\nDependencies\n")
		for _, dependency := range dependencies {
			direction := "upstream"
			otherShotID := dependency.FromShotID
			if dependency.FromShotID == shot.ID {
				direction = "downstream"
				otherShotID = dependency.ToShotID
			}
			fmt.Fprintf(&b, "- %s shot=%s type=%s phase=%s role=%s reason=%s\n",
				direction,
				uuidString(otherShotID),
				dependency.DependencyType,
				dependency.BlockingPhase,
				dependency.InjectionRole,
				dependency.Reason,
			)
		}
	}
	if len(sourceMaterials) == 0 {
		fmt.Fprintf(&b, "\nSource Materials\n- none\n")
	} else {
		fmt.Fprintf(&b, "\nSource Materials\n")
		for _, node := range sourceMaterials {
			writeNodeState(&b, node)
		}
	}
	return b.String()
}

func buildKeyElementStateContextText(input GraphInput, state db.KeyElementState, sourceMaterials []NodeState) string {
	var b strings.Builder
	writeTaskContext(&b, input)
	fmt.Fprintf(&b, "KeyElementState\n")
	fmt.Fprintf(&b, "- id: %s\n", uuidString(state.ID))
	fmt.Fprintf(&b, "- client_key: %s\n", state.ClientKey)
	fmt.Fprintf(&b, "- label: %s\n", state.Label)
	fmt.Fprintf(&b, "- reference_status: %s\n", state.ReferenceStatus)
	fmt.Fprintf(&b, "- visual_description: %s\n", state.VisualDescription)
	if len(sourceMaterials) == 0 {
		fmt.Fprintf(&b, "\nSource Materials\n- none\n")
	} else {
		fmt.Fprintf(&b, "\nSource Materials\n")
		for _, node := range sourceMaterials {
			writeNodeState(&b, node)
		}
	}
	return b.String()
}

func writeTaskContext(b *strings.Builder, input GraphInput) {
	if strings.TrimSpace(input.Mode) == "" && strings.TrimSpace(input.ExecutionPolicy) == "" {
		return
	}
	fmt.Fprintf(b, "Current Task\n")
	if strings.TrimSpace(input.Mode) != "" {
		fmt.Fprintf(b, "- target_phase: %s\n", input.Mode)
	}
	if strings.TrimSpace(input.ExecutionPolicy) != "" {
		fmt.Fprintf(b, "- execution_policy: %s\n", input.ExecutionPolicy)
	}
	fmt.Fprintf(b, "\n")
}

func writeNodeState(b *strings.Builder, node NodeState) {
	fmt.Fprintf(b, "- %s (%s, %s)", node.Node.Title, node.Node.NodeType, node.Node.Status)
	if len(node.Jobs) > 0 {
		latest := node.Jobs[len(node.Jobs)-1]
		fmt.Fprintf(b, " latest_job=%s/%s", latest.OperationType, latest.Status)
	}
	if len(node.Versions) > 0 {
		latest := node.Versions[len(node.Versions)-1]
		fmt.Fprintf(b, " latest_version=%s", latest.Status)
	}
	fmt.Fprintf(b, "\n")
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
