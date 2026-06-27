package reviewer

import (
	"fmt"
	"strings"
)

var allRubricAxes = map[string]bool{
	AxisFaithfulness:          true,
	AxisSubjectConsistency:    true,
	AxisProductVisibility:     true,
	AxisBrandStyleConsistency: true,
	AxisCompositionProportion: true,
	AxisMotionPhysics:         true,
	AxisVisualQuality:         true,
	AxisContinuity:            true,
	AxisAudioSync:             true,
	AxisPlatformSellingPower:  true,
}

var issueDimensions = map[string]bool{
	AxisFaithfulness:          true,
	AxisSubjectConsistency:    true,
	AxisProductVisibility:     true,
	AxisBrandStyleConsistency: true,
	AxisCompositionProportion: true,
	AxisMotionPhysics:         true,
	AxisVisualQuality:         true,
	AxisContinuity:            true,
	AxisAudioSync:             true,
	AxisPlatformSellingPower:  true,
	"model_capability":        true,
	"prompt_validity":         true,
	"reference_role_validity": true,
	"cost_risk":               true,
	"dependency_not_ready":    true,
	"project_memory_conflict": true,
}

func RequiredAxesForReviewTask(task string) []string {
	switch task {
	case ReviewTaskPreRenderPlan:
		return []string{AxisFaithfulness, AxisSubjectConsistency, AxisContinuity}
	case ReviewTaskPreviewImage:
		return []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality}
	case ReviewTaskShotVideo:
		return []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality, AxisMotionPhysics, AxisContinuity, AxisAudioSync}
	case ReviewTaskFinalVideo:
		return []string{AxisFaithfulness, AxisBrandStyleConsistency, AxisVisualQuality, AxisContinuity, AxisAudioSync, AxisPlatformSellingPower}
	default:
		return nil
	}
}

func DefaultReviewPolicy() ReviewPolicy {
	return DefaultReviewPolicyForTask(ReviewTaskPreviewImage)
}

func DefaultReviewPolicyForTask(task string) ReviewPolicy {
	axes := RequiredAxesForReviewTask(task)
	if len(axes) == 0 {
		axes = RequiredAxesForReviewTask(ReviewTaskPreviewImage)
	}
	return ReviewPolicy{
		OverallThreshold: 0.75,
		AxisThreshold:    0.70,
		RequiredAxes:     axes,
		MaxAttempts:      3,
	}
}

func ValidateRubric(result ReviewResult, policy ReviewPolicy) (ReviewDecision, error) {
	task := strings.TrimSpace(result.ReviewTask)
	if task == "" {
		task = ReviewTaskPreviewImage
	}
	if len(policy.RequiredAxes) == 0 {
		policy.RequiredAxes = RequiredAxesForReviewTask(task)
	}
	if policy.OverallThreshold <= 0 {
		defaultPolicy := DefaultReviewPolicyForTask(task)
		policy.OverallThreshold = defaultPolicy.OverallThreshold
		if policy.AxisThreshold <= 0 {
			policy.AxisThreshold = defaultPolicy.AxisThreshold
		}
		if len(policy.RequiredAxes) == 0 {
			policy.RequiredAxes = defaultPolicy.RequiredAxes
		}
	}
	if policy.AxisThreshold <= 0 {
		policy.AxisThreshold = DefaultReviewPolicyForTask(task).AxisThreshold
	}
	if len(policy.RequiredAxes) == 0 {
		return ReviewDecision{}, fmt.Errorf("%w: unsupported review_task %q", ErrInvalidRubric, task)
	}
	if result.Rubric == nil {
		return ReviewDecision{}, fmt.Errorf("%w: rubric is required", ErrInvalidRubric)
	}
	if result.OverallScore < 0 || result.OverallScore > 1 {
		return ReviewDecision{}, fmt.Errorf("%w: overall_score out of range", ErrInvalidRubric)
	}

	verdict := strings.TrimSpace(result.Verdict)
	if verdict == "" {
		verdict = ReviewStatusAccepted
	}
	if !validVerdict(verdict) {
		return ReviewDecision{}, fmt.Errorf("%w: unsupported verdict %q", ErrInvalidRubric, verdict)
	}
	decision := ReviewDecision{Status: verdict}
	for _, axisName := range policy.RequiredAxes {
		axis, ok := result.Rubric[axisName]
		if !ok {
			return ReviewDecision{}, fmt.Errorf("%w: missing axis %s", ErrInvalidRubric, axisName)
		}
		if !allRubricAxes[axisName] {
			return ReviewDecision{}, fmt.Errorf("%w: unsupported axis %s", ErrInvalidRubric, axisName)
		}
		if axis.Score < 0 || axis.Score > 1 {
			return ReviewDecision{}, fmt.Errorf("%w: axis %s score out of range", ErrInvalidRubric, axisName)
		}
		if !axis.Pass || axis.Score < policy.AxisThreshold {
			if decision.Status == ReviewStatusAccepted {
				decision.Status = ReviewStatusRejected
			}
			decision.ShouldRetry = true
			reason := strings.TrimSpace(axis.Reason)
			if reason == "" {
				reason = fmt.Sprintf("%s did not pass", axisName)
			}
			decision.Reasons = append(decision.Reasons, reason)
			if hint := strings.TrimSpace(axis.FixHint); hint != "" {
				decision.FixHints = append(decision.FixHints, hint)
			}
		}
	}
	for axisName, axis := range result.Rubric {
		if !allRubricAxes[axisName] {
			return ReviewDecision{}, fmt.Errorf("%w: unsupported axis %s", ErrInvalidRubric, axisName)
		}
		if axis.Score < 0 || axis.Score > 1 {
			return ReviewDecision{}, fmt.Errorf("%w: axis %s score out of range", ErrInvalidRubric, axisName)
		}
	}
	if result.OverallScore < policy.OverallThreshold && decision.Status == ReviewStatusAccepted {
		decision.Status = ReviewStatusRejected
		decision.ShouldRetry = true
		decision.Reasons = append(decision.Reasons, "overall score below threshold")
	}
	if err := validateIssuesForVerdict(result, decision.Status); err != nil {
		return ReviewDecision{}, err
	}
	for _, hint := range result.RetryRecommendation.FixHints {
		hint = strings.TrimSpace(hint)
		if hint != "" {
			decision.FixHints = append(decision.FixHints, hint)
		}
	}
	if result.RetryRecommendation.ShouldRetry || result.RetryRecommendation.ShouldRepair {
		decision.ShouldRetry = decision.Status == ReviewStatusRejected || decision.Status == ReviewStatusAcceptedWithWarnings
	}
	return decision, nil
}

func validVerdict(value string) bool {
	switch value {
	case ReviewStatusAccepted, ReviewStatusAcceptedWithWarnings, ReviewStatusRejected, ReviewStatusBlocked:
		return true
	default:
		return false
	}
}

func validateIssuesForVerdict(result ReviewResult, status string) error {
	hasBlocking := false
	for _, issue := range result.Issues {
		if !issueDimensions[issue.Dimension] {
			return fmt.Errorf("%w: unsupported issue dimension %s", ErrInvalidRubric, issue.Dimension)
		}
		if issue.Severity != IssueSeverityInfo && issue.Severity != IssueSeverityWarning && issue.Severity != IssueSeverityBlocking {
			return fmt.Errorf("%w: unsupported issue severity %s", ErrInvalidRubric, issue.Severity)
		}
		if issue.Severity == IssueSeverityBlocking {
			hasBlocking = true
		}
	}
	switch status {
	case ReviewStatusRejected:
		if !hasBlocking {
			return fmt.Errorf("%w: rejected review requires at least one blocking issue", ErrInvalidRubric)
		}
	case ReviewStatusAccepted:
		if hasBlocking {
			return fmt.Errorf("%w: accepted review cannot include blocking issue", ErrInvalidRubric)
		}
	case ReviewStatusBlocked:
		if strings.TrimSpace(result.Critique) == "" || strings.TrimSpace(result.Reason) == "" {
			return fmt.Errorf("%w: blocked review requires critique and reason", ErrInvalidRubric)
		}
	}
	return nil
}
