#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export CLIPANVIL_E2E_PRODUCER_FIXTURE=motion_shot_video
export CLIPANVIL_E2E_CRAFTSMAN_FIXTURE=motion_shot_video
export CLIPANVIL_E2E_COMPOSER_FIXTURE=motion_shot_video
export CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1

echo "[m12] dev env"
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh

echo "[m12] targeted tests"
(cd apps/server && go test ./cmd/server -run 'TestMotionShotVideoFixturePlansAudioAndMotionShotVideo|TestMotionShotVideoCraftsmanFixtureVariesPlansByShotFacts|TestMotionShotVideoComposerFixtureBuildsThirtyFourSecondMultiShotTimeline|TestMotionShotVideoFixtureDispatchesAudioOnContinuationMessages|TestMotionShotVideoFixturePrioritizesFinalCompositionOverMotionShotContinuation' -count=1)
(cd apps/server && go test ./internal/agent/tools -run 'TestDispatchCraftsmanMotionOnlyPolicy' -count=1)
(cd apps/server && go test ./internal/motionshot -count=1)

cat <<'EOF'
[m12] Browser smoke steps:
1. Start the app with ./scripts/dev-start.sh using the env above.
2. Open the printed Vite URL.
3. Create an Agent workspace.
4. Upload /Users/wanwan/Desktop/box.png.
5. Ask: 用这张图生成一个 30 秒以上的悦行行李箱口播广告。不要调用 Seedance；图片可以用 Seedream；旁白和 BGM 用火山；视频用 Remotion 图片动效；需要多分镜、转场、字幕和最终成片。
6. Continue until final_video is completed.
7. Verify DB has at least 5 shots, motion_shot_video plans, no Seedance generation jobs, and final MP4 duration >= 30.
EOF
