package identity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func RenderObjectIndex(rows []db.AgentObjectIndex) string {
	if len(rows) == 0 {
		return "可操作对象索引：当前没有可操作对象。"
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ObjectType != rows[j].ObjectType {
			return objectRank(rows[i].ObjectType) < objectRank(rows[j].ObjectType)
		}
		if rows[i].SortOrder != rows[j].SortOrder {
			return rows[i].SortOrder < rows[j].SortOrder
		}
		return rows[i].SemanticKey < rows[j].SemanticKey
	})
	lines := []string{"可操作对象索引："}
	for _, row := range rows {
		if strings.TrimSpace(row.SemanticKey) == "" {
			continue
		}
		switch row.ObjectType {
		case ObjectCreativeBrief:
			lines = append(lines, fmt.Sprintf("- CreativeBrief %s｜%s｜%s", row.SemanticKey, row.DisplayName, row.Status))
		case ObjectProjectMemory:
			lines = append(lines, fmt.Sprintf("- ProjectMemory %s｜%s｜%s", row.SemanticKey, row.DisplayName, row.Status))
		case ObjectKeyElement:
			lines = append(lines, fmt.Sprintf("- KeyElement %s｜%s｜%s｜%s", row.SemanticKey, row.DisplayName, row.Kind, row.Status))
		case ObjectKeyElementState:
			lines = append(lines, fmt.Sprintf("  - ElementState %s｜%s｜reference=%s｜parent=%s", row.SemanticKey, row.DisplayName, row.Kind, row.ParentSemanticKey))
		case ObjectScene:
			lines = append(lines, fmt.Sprintf("- Scene %s｜%s｜%s", row.SemanticKey, row.DisplayName, row.Status))
		case ObjectShot:
			lines = append(lines, fmt.Sprintf("  - Shot %s｜%s｜%s｜parent=%s", row.SemanticKey, row.DisplayName, row.Status, row.ParentSemanticKey))
		case ObjectShotDependency:
			lines = append(lines, fmt.Sprintf("    - Dependency %s｜%s｜%s｜target=%s", row.SemanticKey, row.DisplayName, row.Kind, row.ParentSemanticKey))
		case ObjectRenderPlan:
			lines = append(lines, fmt.Sprintf("    - RenderPlan %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectMediaNode:
			lines = append(lines, fmt.Sprintf("      - MediaNode %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectGenerationJob:
			lines = append(lines, fmt.Sprintf("      - GenerationJob %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectArtifactVersion:
			lines = append(lines, fmt.Sprintf("        - Artifact %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectReviewRecord:
			lines = append(lines, fmt.Sprintf("      - Review %s｜%s｜%s｜target=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectArtifactIssue:
			lines = append(lines, fmt.Sprintf("      - Issue %s｜%s｜%s｜target=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		}
	}
	return strings.Join(lines, "\n")
}

func objectRank(objectType string) int {
	switch objectType {
	case ObjectCreativeBrief:
		return 0
	case ObjectProjectMemory:
		return 1
	case ObjectKeyElement:
		return 2
	case ObjectKeyElementState:
		return 3
	case ObjectScene:
		return 4
	case ObjectShot:
		return 5
	case ObjectShotDependency:
		return 6
	case ObjectRenderPlan:
		return 7
	case ObjectMediaNode:
		return 8
	case ObjectGenerationJob:
		return 9
	case ObjectArtifactVersion:
		return 10
	case ObjectReviewRecord:
		return 11
	case ObjectArtifactIssue:
		return 12
	case ObjectAgentThread:
		return 13
	case ObjectAgentTask:
		return 14
	case ObjectProducerPendingSignal:
		return 15
	default:
		return 100
	}
}
