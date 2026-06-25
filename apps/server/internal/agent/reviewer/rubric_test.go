package reviewer

import (
	"reflect"
	"testing"
)

func TestRequiredAxesByReviewTask(t *testing.T) {
	tests := []struct {
		task string
		want []string
	}{
		{ReviewTaskPreRenderPlan, []string{AxisFaithfulness, AxisSubjectConsistency, AxisContinuity}},
		{ReviewTaskPreviewImage, []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality}},
		{ReviewTaskShotVideo, []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality, AxisMotionPhysics, AxisContinuity, AxisAudioSync}},
		{ReviewTaskFinalVideo, []string{AxisFaithfulness, AxisBrandStyleConsistency, AxisVisualQuality, AxisContinuity, AxisAudioSync, AxisPlatformSellingPower}},
	}
	for _, tt := range tests {
		got := RequiredAxesForReviewTask(tt.task)
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s axes = %#v, want %#v", tt.task, got, tt.want)
		}
	}
}

func TestValidateRubricAcceptsPassingPreviewReview(t *testing.T) {
	result := ReviewResult{
		ReviewTask:   ReviewTaskPreviewImage,
		Verdict:      ReviewStatusAccepted,
		OverallScore: 0.86,
		Rubric:       passingRequiredAxes(ReviewTaskPreviewImage),
		Critique:     "可用",
	}

	decision, err := ValidateRubric(result, DefaultReviewPolicyForTask(ReviewTaskPreviewImage))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != ReviewStatusAccepted {
		t.Fatalf("status = %q, want accepted", decision.Status)
	}
	if decision.ShouldRetry {
		t.Fatal("passing review should not retry")
	}
}

func TestValidateRubricRejectsLowRequiredAxis(t *testing.T) {
	result := ReviewResult{
		ReviewTask:   ReviewTaskShotVideo,
		Verdict:      ReviewStatusRejected,
		OverallScore: 0.62,
		Rubric:       passingRequiredAxes(ReviewTaskShotVideo),
		Critique:     "商品颜色漂移，动作不合理。",
		Issues: []ReviewIssue{{
			Dimension:        AxisVisualQuality,
			Severity:         IssueSeverityBlocking,
			Title:            "画面质量不足",
			Description:      "商品特写模糊，不能进入剪辑。",
			TargetObjectType: "artifact_version",
			TargetObjectID:   "01000000-0000-0000-0000-000000000000",
			SuggestedFix:     "regenerate",
			FixHint:          "重新生成更清晰的商品特写。",
		}},
	}
	axis := result.Rubric[AxisVisualQuality]
	axis.Score = 0.42
	axis.Pass = false
	axis.FixHint = "重新生成更清晰的商品特写"
	result.Rubric[AxisVisualQuality] = axis

	decision, err := ValidateRubric(result, DefaultReviewPolicyForTask(ReviewTaskShotVideo))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != ReviewStatusRejected {
		t.Fatalf("status = %q, want rejected", decision.Status)
	}
	if !decision.ShouldRetry {
		t.Fatal("low required axis should recommend retry")
	}
	if len(decision.FixHints) == 0 || decision.FixHints[0] != "重新生成更清晰的商品特写" {
		t.Fatalf("fix hints = %#v", decision.FixHints)
	}
}

func TestValidateRubricFailsMissingRequiredAxis(t *testing.T) {
	result := ReviewResult{
		ReviewTask:   ReviewTaskFinalVideo,
		Verdict:      ReviewStatusAccepted,
		OverallScore: 0.86,
		Rubric:       passingRequiredAxes(ReviewTaskFinalVideo),
		Critique:     "可用",
	}
	delete(result.Rubric, AxisPlatformSellingPower)

	_, err := ValidateRubric(result, DefaultReviewPolicyForTask(ReviewTaskFinalVideo))
	if err == nil {
		t.Fatal("expected missing required axis to fail validation")
	}
}

func TestValidateReviewResultRequiresBlockingIssueOnRejected(t *testing.T) {
	result := ReviewResult{
		ReviewTask:   ReviewTaskShotVideo,
		Verdict:      ReviewVerdictRejected,
		OverallScore: 0.4,
		Rubric:       passingRequiredAxes(ReviewTaskShotVideo),
		Critique:     "商品颜色漂移，不能进入剪辑。",
	}
	_, err := ValidateRubric(result, DefaultReviewPolicyForTask(ReviewTaskShotVideo))
	if err == nil {
		t.Fatal("expected rejected result without blocking issue to fail")
	}
}

func passingRequiredAxes(task string) map[string]RubricAxis {
	out := map[string]RubricAxis{}
	for _, axis := range RequiredAxesForReviewTask(task) {
		out[axis] = RubricAxis{Score: 0.88, Pass: true, Severity: IssueSeverityInfo, Reason: "通过"}
	}
	return out
}

func passingReviewResult() ReviewResult {
	return ReviewResult{
		ReviewTask:   ReviewTaskPreviewImage,
		Verdict:      ReviewStatusAccepted,
		OverallScore: 0.86,
		Rubric:       passingRequiredAxes(ReviewTaskPreviewImage),
		Critique:     "可用",
	}
}
