package reviewer

import (
	"fmt"
	"strings"
)

var RequiredPreviewAxes = []string{
	"proportion",
	"physics",
	"style",
	"visual_quality",
	"product_visibility",
	"selling_power",
	"platform_fit",
}

func DefaultReviewPolicy() ReviewPolicy {
	return ReviewPolicy{
		OverallThreshold: 0.75,
		AxisThreshold:    0.70,
		RequiredAxes:     RequiredPreviewAxes,
		MaxAttempts:      3,
	}
}

func ValidateRubric(result ReviewResult, policy ReviewPolicy) (ReviewDecision, error) {
	if policy.OverallThreshold <= 0 {
		policy = DefaultReviewPolicy()
	}
	if len(policy.RequiredAxes) == 0 {
		policy.RequiredAxes = RequiredPreviewAxes
	}
	if result.Rubric == nil {
		return ReviewDecision{}, fmt.Errorf("%w: rubric is required", ErrInvalidRubric)
	}
	if result.OverallScore < 0 || result.OverallScore > 1 {
		return ReviewDecision{}, fmt.Errorf("%w: overall_score out of range", ErrInvalidRubric)
	}

	decision := ReviewDecision{Status: ReviewStatusAccepted}
	for _, axisName := range policy.RequiredAxes {
		axis, ok := result.Rubric[axisName]
		if !ok {
			return ReviewDecision{}, fmt.Errorf("%w: missing axis %s", ErrInvalidRubric, axisName)
		}
		if axis.Score < 0 || axis.Score > 1 {
			return ReviewDecision{}, fmt.Errorf("%w: axis %s score out of range", ErrInvalidRubric, axisName)
		}
		if !axis.Pass || axis.Score < policy.AxisThreshold {
			decision.Status = ReviewStatusRejected
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
	if result.OverallScore < policy.OverallThreshold {
		decision.Status = ReviewStatusRejected
		decision.ShouldRetry = true
		decision.Reasons = append(decision.Reasons, "overall score below threshold")
	}
	for _, hint := range result.RetryRecommendation.FixHints {
		hint = strings.TrimSpace(hint)
		if hint != "" {
			decision.FixHints = append(decision.FixHints, hint)
		}
	}
	if result.RetryRecommendation.ShouldRetry && decision.Status == ReviewStatusRejected {
		decision.ShouldRetry = true
	}
	return decision, nil
}
