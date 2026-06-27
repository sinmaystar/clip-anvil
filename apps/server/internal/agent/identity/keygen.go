package identity

import (
	"fmt"
	"regexp"
	"strings"
)

var nonKeyPart = regexp.MustCompile(`[^a-z0-9]+`)

func NormalizeKeyPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonKeyPart.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "item"
	}
	return value
}

func SceneKey(title string, sortOrder int) string {
	part := NormalizeKeyPart(title)
	if part == "item" {
		return fmt.Sprintf("scene_%02d", sortOrder)
	}
	return "scene_" + part
}

func ShotKey(sortOrder int) string {
	if sortOrder < 1 {
		sortOrder = 1
	}
	return fmt.Sprintf("shot_%02d", sortOrder)
}

func KeyElementKey(name string) string {
	return "element_" + NormalizeKeyPart(name)
}

func KeyElementStateKey(elementKey string, label string) string {
	elementKey = strings.TrimSpace(elementKey)
	if elementKey == "" {
		elementKey = "element_item"
	}
	return elementKey + ".state_" + NormalizeKeyPart(label)
}

func RenderPlanKey(scopeKey string, targetPhase string, revision int32) string {
	if revision < 1 {
		revision = 1
	}
	return fmt.Sprintf("%s.%s.rp%d", strings.TrimSpace(scopeKey), NormalizeKeyPart(targetPhase), revision)
}

func MediaNodeKey(renderPlanKey string) string {
	return strings.TrimSpace(renderPlanKey) + ".output"
}

func GenerationJobKey(renderPlanKey string, attempt int32) string {
	if attempt < 1 {
		attempt = 1
	}
	return fmt.Sprintf("%s.job%d", strings.TrimSpace(renderPlanKey), attempt)
}

func ArtifactVersionKey(mediaNodeKey string, versionNo int32) string {
	if versionNo < 1 {
		versionNo = 1
	}
	return fmt.Sprintf("%s.v%d", strings.TrimSpace(mediaNodeKey), versionNo)
}
