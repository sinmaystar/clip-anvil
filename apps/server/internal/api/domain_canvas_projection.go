package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type domainCanvasProjectionResponse struct {
	Nodes []domainCanvasNodeResponse `json:"nodes"`
	Edges []domainCanvasEdgeResponse `json:"edges"`
}

type domainCanvasNodeResponse struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Title    string            `json:"title"`
	Subtitle string            `json:"subtitle,omitempty"`
	Status   string            `json:"status,omitempty"`
	X        float32           `json:"x"`
	Y        float32           `json:"y"`
	W        float32           `json:"w"`
	H        float32           `json:"h"`
	Meta     map[string]string `json:"meta,omitempty"`
}

type domainCanvasEdgeResponse struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
}

func buildDomainCanvasProjection(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (domainCanvasProjectionResponse, error) {
	if queries == nil || !workspaceID.Valid {
		return domainCanvasProjectionResponse{}, nil
	}
	projection := domainCanvasProjectionResponse{}
	brief, hasBrief, err := activeCreativeBrief(ctx, queries, workspaceID)
	if err != nil {
		return projection, err
	}
	if hasBrief {
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       "domain:creative_brief:" + uuidToString(brief.ID),
			Kind:     "creative_brief",
			Title:    brief.Title,
			Subtitle: brief.Concept,
			Status:   brief.Status,
			X:        -520,
			Y:        -260,
			W:        260,
			H:        132,
		})
	}
	memory, hasMemory, err := activeProjectMemory(ctx, queries, workspaceID)
	if err != nil {
		return projection, err
	}
	if hasMemory {
		memoryID := "domain:project_memory:" + uuidToString(memory.ID)
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       memoryID,
			Kind:     "project_memory",
			Title:    fmt.Sprintf("ProjectMemory v%d", memory.Version),
			Subtitle: memory.Soul,
			Status:   memory.Status,
			X:        -220,
			Y:        -260,
			W:        260,
			H:        132,
		})
		if hasBrief {
			projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
				ID:     "domain:edge:brief-memory",
				Kind:   "domain_reference",
				Source: "domain:creative_brief:" + uuidToString(brief.ID),
				Target: memoryID,
				Label:  "约束",
			})
		}
	}
	elements, err := queries.ListActiveKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	states, err := queries.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	elementX := float32(-520)
	scopeYByNodeID := map[string]float32{}
	for i, element := range elements {
		nodeID := "domain:key_element:" + uuidToString(element.ID)
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       nodeID,
			Kind:     "key_element",
			Title:    element.Name,
			Subtitle: element.ClientKey,
			Status:   element.Status,
			X:        elementX + float32(i%2)*300,
			Y:        40 + float32(i/2)*170,
			W:        260,
			H:        132,
			Meta:     map[string]string{"type": element.ElementType},
		})
		for j, state := range states {
			if state.KeyElementID != element.ID {
				continue
			}
			stateID := "domain:key_element_state:" + uuidToString(state.ID)
			stateY := float32(190 + float32(i/2)*170 + float32(j)*150)
			scopeYByNodeID[stateID] = stateY
			projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
				ID:       stateID,
				Kind:     "key_element_state",
				Title:    state.Label,
				Subtitle: state.ClientKey,
				Status:   state.ReferenceStatus,
				X:        elementX + float32(i%2)*300,
				Y:        stateY,
				W:        260,
				H:        120,
			})
			projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
				ID:     "domain:edge:" + uuidToString(element.ID) + ":" + uuidToString(state.ID),
				Kind:   "state",
				Source: nodeID,
				Target: stateID,
				Label:  "state",
			})
		}
	}
	scenes, err := queries.ListActiveScenesByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	shots, err := queries.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	sceneIDByUUID := map[pgtype.UUID]string{}
	for i, scene := range scenes {
		nodeID := "domain:scene:" + uuidToString(scene.ID)
		sceneIDByUUID[scene.ID] = nodeID
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       nodeID,
			Kind:     "scene",
			Title:    scene.Title,
			Subtitle: scene.Location,
			Status:   scene.Status,
			X:        160,
			Y:        -260 + float32(i)*180,
			W:        280,
			H:        132,
		})
	}
	shotIDByUUID := map[pgtype.UUID]string{}
	for i, shot := range shots {
		nodeID := "domain:shot:" + uuidToString(shot.ID)
		shotIDByUUID[shot.ID] = nodeID
		shotY := float32(-260 + float32(i)*170)
		scopeYByNodeID[nodeID] = shotY
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       nodeID,
			Kind:     "shot",
			Title:    shot.Title,
			Subtitle: shot.CreativeText,
			Status:   shot.Status,
			X:        500,
			Y:        shotY,
			W:        300,
			H:        140,
			Meta:     map[string]string{"client_key": shot.ClientKey},
		})
		if sceneNodeID := sceneIDByUUID[shot.SceneID]; sceneNodeID != "" {
			projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
				ID:     "domain:edge:" + uuidToString(shot.SceneID) + ":" + uuidToString(shot.ID),
				Kind:   "contains",
				Source: sceneNodeID,
				Target: nodeID,
				Label:  "shot",
			})
		}
	}
	links, err := queries.ListShotKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	for _, link := range links {
		source := shotIDByUUID[link.ShotID]
		target := ""
		if link.KeyElementStateID.Valid {
			target = "domain:key_element_state:" + uuidToString(link.KeyElementStateID)
		} else {
			target = "domain:key_element:" + uuidToString(link.KeyElementID)
		}
		if source == "" || target == "" {
			continue
		}
		projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
			ID:     "domain:edge:" + uuidToString(link.ID),
			Kind:   "shot_key_element",
			Source: source,
			Target: target,
			Label:  link.Role,
		})
	}
	deps, err := queries.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	for _, dep := range deps {
		source := shotIDByUUID[dep.FromShotID]
		target := shotIDByUUID[dep.ToShotID]
		if source == "" || target == "" {
			continue
		}
		projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
			ID:     "domain:edge:" + uuidToString(dep.ID),
			Kind:   "shot_dependency",
			Source: source,
			Target: target,
			Label:  dep.DependencyType,
		})
	}
	renderPlans, err := queries.ListRenderPlansByWorkspace(ctx, workspaceID)
	if err != nil {
		return projection, err
	}
	renderPlanYByScope := map[string]int{}
	renderPlanIDByUUID := map[pgtype.UUID]string{}
	for _, plan := range renderPlans {
		source := ""
		switch plan.ScopeType {
		case "shot":
			source = shotIDByUUID[plan.ScopeID]
		case "key_element_state":
			source = "domain:key_element_state:" + uuidToString(plan.ScopeID)
		}
		if source == "" {
			continue
		}
		scopeOffset := renderPlanYByScope[source]
		renderPlanYByScope[source] = scopeOffset + 1
		nodeID := "domain:render_plan:" + uuidToString(plan.ID)
		renderPlanIDByUUID[plan.ID] = nodeID
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       nodeID,
			Kind:     "render_plan",
			Title:    fmt.Sprintf("RenderPlan r%d", plan.Revision),
			Subtitle: plan.TargetPhase,
			Status:   plan.Status,
			X:        860,
			Y:        scopeYByNodeID[source] + float32(scopeOffset)*150,
			W:        280,
			H:        132,
			Meta: map[string]string{
				"profile":        plan.ModelPromptProfile,
				"operation":      plan.Operation,
				"compiled_chars": fmt.Sprintf("%d", len([]rune(plan.CompiledPrompt))),
			},
		})
		projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
			ID:     "domain:edge:render_plan:" + uuidToString(plan.ID),
			Kind:   "render_plan",
			Source: source,
			Target: nodeID,
			Label:  plan.TargetPhase,
		})
	}
	reviews, err := queries.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return projection, err
	}
	reviewIDByUUID := map[pgtype.UUID]string{}
	for i, review := range reviews {
		nodeID := "domain:review_record:" + uuidToString(review.ID)
		reviewIDByUUID[review.ID] = nodeID
		title := review.ReviewTask
		if title == "" {
			title = review.TargetPhase
		}
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       nodeID,
			Kind:     "review_record",
			Title:    title,
			Subtitle: fmt.Sprintf("score=%s", reviewScoreText(review.OverallScore)),
			Status:   review.Status,
			X:        1180,
			Y:        -260 + float32(i)*150,
			W:        260,
			H:        120,
			Meta: map[string]string{
				"target_phase": review.TargetPhase,
				"critique":     review.Critique,
			},
		})
		target := ""
		if review.RenderPlanID.Valid {
			target = renderPlanIDByUUID[review.RenderPlanID]
		}
		if target == "" && review.ShotID.Valid {
			target = shotIDByUUID[review.ShotID]
		}
		if target != "" {
			projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
				ID:     "domain:edge:review:" + uuidToString(review.ID),
				Kind:   "reviews",
				Source: nodeID,
				Target: target,
				Label:  "reviews",
			})
		}
	}
	issues, err := queries.ListOpenArtifactIssuesByWorkspace(ctx, db.ListOpenArtifactIssuesByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return projection, err
	}
	for i, issue := range issues {
		nodeID := "domain:artifact_issue:" + uuidToString(issue.ID)
		projection.Nodes = append(projection.Nodes, domainCanvasNodeResponse{
			ID:       nodeID,
			Kind:     "artifact_issue",
			Title:    issue.Title,
			Subtitle: issue.Dimension,
			Status:   issue.Severity,
			X:        1480,
			Y:        -260 + float32(i)*140,
			W:        260,
			H:        120,
			Meta: map[string]string{
				"suggested_fix": issue.SuggestedFix,
				"fix_hint":      issue.FixHint,
			},
		})
		if source := reviewIDByUUID[issue.ReviewRecordID]; source != "" {
			projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
				ID:     "domain:edge:flags:" + uuidToString(issue.ID),
				Kind:   "flags",
				Source: source,
				Target: nodeID,
				Label:  issue.Severity,
			})
		}
		target := issueTargetNodeID(issue, renderPlanIDByUUID, shotIDByUUID)
		if target != "" {
			projection.Edges = append(projection.Edges, domainCanvasEdgeResponse{
				ID:     "domain:edge:suggests_fix:" + uuidToString(issue.ID),
				Kind:   "suggests_fix",
				Source: nodeID,
				Target: target,
				Label:  issue.SuggestedFix,
			})
		}
	}
	return projection, nil
}

func issueTargetNodeID(issue db.ArtifactIssue, renderPlans map[pgtype.UUID]string, shots map[pgtype.UUID]string) string {
	switch issue.TargetObjectType {
	case "render_plan":
		return renderPlans[issue.TargetObjectID]
	case "shot":
		return shots[issue.TargetObjectID]
	default:
		return ""
	}
}

func reviewScoreText(score pgtype.Float4) string {
	if !score.Valid {
		return "-"
	}
	return fmt.Sprintf("%.2f", score.Float32)
}

func activeCreativeBrief(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (db.CreativeBrief, bool, error) {
	brief, err := queries.GetActiveCreativeBriefByWorkspace(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CreativeBrief{}, false, nil
	}
	return brief, err == nil, err
}

func activeProjectMemory(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (db.ProjectMemory, bool, error) {
	memory, err := queries.GetActiveProjectMemoryByWorkspace(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ProjectMemory{}, false, nil
	}
	return memory, err == nil, err
}
