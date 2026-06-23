package reviewer

import "testing"

func TestValidateRubricAcceptsPassingPreviewReview(t *testing.T) {
	result := ReviewResult{
		OverallScore: 0.86,
		Rubric: map[string]RubricAxis{
			"proportion":         {Score: 0.82, Pass: true, Reason: "主体比例合理"},
			"physics":            {Score: 0.80, Pass: true, Reason: "光影可信"},
			"style":              {Score: 0.84, Pass: true, Reason: "匹配风格"},
			"visual_quality":     {Score: 0.91, Pass: true, Reason: "清晰"},
			"product_visibility": {Score: 0.88, Pass: true, Reason: "商品清楚"},
			"selling_power":      {Score: 0.78, Pass: true, Reason: "支持卖点"},
			"platform_fit":       {Score: 0.81, Pass: true, Reason: "适合短视频"},
		},
		Critique: "可用",
	}

	decision, err := ValidateRubric(result, DefaultReviewPolicy())
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
	result := passingReviewResult()
	axis := result.Rubric["visual_quality"]
	axis.Score = 0.42
	axis.Pass = false
	axis.FixHint = "重新生成更清晰的商品特写"
	result.Rubric["visual_quality"] = axis

	decision, err := ValidateRubric(result, DefaultReviewPolicy())
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
	result := passingReviewResult()
	delete(result.Rubric, "platform_fit")

	_, err := ValidateRubric(result, DefaultReviewPolicy())
	if err == nil {
		t.Fatal("expected missing required axis to fail validation")
	}
}

func passingReviewResult() ReviewResult {
	return ReviewResult{
		OverallScore: 0.86,
		Rubric: map[string]RubricAxis{
			"proportion":         {Score: 0.82, Pass: true, Reason: "主体比例合理"},
			"physics":            {Score: 0.80, Pass: true, Reason: "光影可信"},
			"style":              {Score: 0.84, Pass: true, Reason: "匹配风格"},
			"visual_quality":     {Score: 0.91, Pass: true, Reason: "清晰"},
			"product_visibility": {Score: 0.88, Pass: true, Reason: "商品清楚"},
			"selling_power":      {Score: 0.78, Pass: true, Reason: "支持卖点"},
			"platform_fit":       {Score: 0.81, Pass: true, Reason: "适合短视频"},
		},
		Critique: "可用",
	}
}
