#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_DIR="$ROOT_DIR/sandbox-image/remotion-timeline"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need node
need npm
need ffmpeg
need ffprobe

if [[ ! -d "$PROJECT_DIR/node_modules" ]]; then
  npm --prefix "$PROJECT_DIR" ci --omit=dev
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  local status=$?
  if [[ "$status" -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "smoke failed; keeping temp dir: $TMP_DIR" >&2
  fi
}
trap cleanup EXIT

PUBLIC_DIR="$TMP_DIR/workspace"
mkdir -p "$PUBLIC_DIR/input" "$PUBLIC_DIR/output"

ffmpeg -y -f lavfi -i color=c=0xd8dde7:s=900x1200 -vf "drawbox=x=260:y=420:w=380:h=540:color=0x687381@1:t=fill,drawbox=x=302:y=470:w=296:h=390:color=0xc8ced7@1:t=fill" -frames:v 1 "$PUBLIC_DIR/input/hero-packshot.png" >/dev/null 2>&1
ffmpeg -y -f lavfi -i color=c=0xf4f4f5:s=900x1200 -vf "drawbox=x=255:y=800:w=390:h=160:color=0x737373@1:t=fill,drawbox=x=322:y=918:w=90:h=90:color=0x171717@1:t=fill,drawbox=x=488:y=918:w=90:h=90:color=0x171717@1:t=fill" -frames:v 1 "$PUBLIC_DIR/input/wheel-detail.png" >/dev/null 2>&1
ffmpeg -y -f lavfi -i color=c=0xf8fafc:s=900x1200 -vf "drawbox=x=180:y=560:w=540:h=360:color=0x9ca3af@1:t=fill,drawbox=x=220:y=610:w=220:h=260:color=0xe5e7eb@1:t=fill,drawbox=x=470:y=610:w=210:h=260:color=0xd1d5db@1:t=fill" -frames:v 1 "$PUBLIC_DIR/input/open-storage.png" >/dev/null 2>&1
ffmpeg -y -f lavfi -i sine=frequency=620:duration=14 -ac 2 -ar 48000 "$PUBLIC_DIR/input/voiceover.mp3" >/dev/null 2>&1
ffmpeg -y -f lavfi -i sine=frequency=220:duration=14 -ac 2 -ar 48000 "$PUBLIC_DIR/input/bgm.mp3" >/dev/null 2>&1

cat >"$TMP_DIR/timeline-plan.json" <<'JSON'
{
  "schema": "clipanvil.remotion_timeline.v1",
  "composition": "MarketingTimeline",
  "output": {"width": 1080, "height": 1920, "fps": 30, "duration_sec": 14, "codec": "h264", "audio_codec": "aac"},
  "theme": {"brand_colors": ["#111827", "#F5C542"], "font_family": "Noto Sans CJK SC", "style": "premium_product_ad"},
  "segments": [
    {
      "id": "seg_hero",
      "shot_ref": "shot_01",
      "start_sec": 0,
      "end_sec": 2,
      "layout": "hero_packshot",
      "visual_focus": "悦行行李箱",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/hero-packshot.png"}],
      "motion": {"preset": "push_in", "intensity": 0.5},
      "transition_in": {"type": "crossfade", "duration_sec": 0.25},
      "text_layers": [{"role": "hook", "text": "轻松出发", "start_sec": 0, "end_sec": 2, "position": "upper_third"}],
      "caption": {"source": "audio_cue", "text": "短途出行，轻便好推。", "start_sec": 0, "end_sec": 2, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_detail",
      "shot_ref": "shot_02",
      "start_sec": 2,
      "end_sec": 4,
      "layout": "detail_focus",
      "visual_focus": "顺滑万向轮",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/wheel-detail.png"}],
      "motion": {"preset": "spotlight_reveal", "intensity": 0.65},
      "transition_in": {"type": "slide", "duration_sec": 0.25},
      "caption": {"source": "audio_cue", "text": "顺滑万向轮，转弯更从容。", "start_sec": 2, "end_sec": 4, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_benefit",
      "shot_ref": "shot_03",
      "start_sec": 4,
      "end_sec": 6,
      "layout": "benefit_card",
      "visual_focus": "轻便省力",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/hero-packshot.png"}],
      "motion": {"preset": "float_parallax", "intensity": 0.55},
      "transition_in": {"type": "wipe", "duration_sec": 0.25},
      "text_layers": [{"role": "headline", "text": "推行省力", "start_sec": 4, "end_sec": 6, "position": "upper_third"}],
      "caption": {"source": "audio_cue", "text": "轻装上路，上下班也顺手。", "start_sec": 4, "end_sec": 6, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_split",
      "shot_ref": "shot_04",
      "start_sec": 6,
      "end_sec": 8,
      "layout": "split_compare",
      "visual_focus": "通勤与周末",
      "assets": [
        {"role": "primary", "type": "image", "workspace_path": "/workspace/input/hero-packshot.png"},
        {"role": "secondary", "type": "image", "workspace_path": "/workspace/input/open-storage.png"}
      ],
      "motion": {"preset": "pan_left", "intensity": 0.45},
      "transition_in": {"type": "zoom_blur", "duration_sec": 0.25},
      "caption": {"source": "audio_cue", "text": "通勤和周末，都有合适容量。", "start_sec": 6, "end_sec": 8, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_scenario",
      "shot_ref": "shot_05",
      "start_sec": 8,
      "end_sec": 10,
      "layout": "scenario_card",
      "visual_focus": "短途出差",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/hero-packshot.png"}],
      "motion": {"preset": "pan_right", "intensity": 0.45},
      "transition_in": {"type": "crossfade", "duration_sec": 0.25},
      "caption": {"source": "audio_cue", "text": "出差短住，一箱就够。", "start_sec": 8, "end_sec": 10, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_storage",
      "shot_ref": "shot_06",
      "start_sec": 10,
      "end_sec": 12,
      "layout": "open_storage",
      "visual_focus": "分区收纳",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/open-storage.png"}],
      "motion": {"preset": "pull_out", "intensity": 0.55},
      "transition_in": {"type": "slide", "duration_sec": 0.25},
      "caption": {"source": "audio_cue", "text": "打开一看，分区清楚。", "start_sec": 10, "end_sec": 12, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_cta",
      "shot_ref": "shot_07",
      "start_sec": 12,
      "end_sec": 14,
      "layout": "cta_endcard",
      "visual_focus": "现在出发",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/hero-packshot.png"}],
      "motion": {"preset": "cta_pop", "intensity": 0.75},
      "transition_in": {"type": "cut", "duration_sec": 0},
      "text_layers": [{"role": "cta", "text": "现在出发", "start_sec": 12, "end_sec": 14, "position": "upper_third"}],
      "caption": {"source": "audio_cue", "text": "悦行行李箱，陪你轻松出发。", "start_sec": 12, "end_sec": 14, "position": "subtitle_bottom"}
    }
  ],
  "audio_tracks": [
    {"id": "voiceover_main", "role": "voiceover", "workspace_path": "/workspace/input/voiceover.mp3", "start_sec": 0, "volume": 1},
    {"id": "bgm_main", "role": "bgm", "workspace_path": "/workspace/input/bgm.mp3", "start_sec": 0, "volume": 0.22, "loop": true}
  ],
  "captions": {"source": "audio_cue", "single_lane": true, "max_chars_per_line": 18, "style": "commerce_subtitle"}
}
JSON

OUT="$PUBLIC_DIR/output/final-remotion-layouts.mp4"
node "$PROJECT_DIR/src/render.mjs" --props "$TMP_DIR/timeline-plan.json" --out "$OUT" --public-dir "$PUBLIC_DIR"

video_dims="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$OUT")"
audio_stream="$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_type -of csv=p=0 "$OUT")"
echo "ffprobe video_dims=$video_dims audio_stream=$audio_stream"
grep -Eq '^1080,1920,?$' <<<"$video_dims"
grep -q '^audio$' <<<"$audio_stream"

duration="$(ffprobe -v error -show_entries format=duration -of default=nk=1:nw=1 "$OUT")"
awk -v duration="$duration" 'BEGIN { if (duration < 13.5 || duration > 14.8) exit 1 }'

echo "M13.3 Remotion layout smoke passed: $OUT duration=$duration"
