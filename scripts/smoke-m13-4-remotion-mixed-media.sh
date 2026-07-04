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

ffmpeg -y -f lavfi -i color=c=0x0f172a:s=900x1200:d=5:r=30 \
  -vf "drawbox=x=230:y=330:w=440:h=620:color=0xb8bec8@1:t=fill,drawbox=x=275:y=390:w=350:h=500:color=0xe5e7eb@1:t=fill,drawbox=x=310+80*sin(t*2):y=910:w=80:h=80:color=0x111827@1:t=fill,drawbox=x=510+80*sin(t*2):y=910:w=80:h=80:color=0x111827@1:t=fill" \
  -an "$PUBLIC_DIR/input/hero-seedance.mp4" >/dev/null 2>&1
ffmpeg -y -f lavfi -i color=c=0xf8fafc:s=900x1200 \
  -vf "drawbox=x=255:y=800:w=390:h=160:color=0x737373@1:t=fill,drawbox=x=322:y=918:w=90:h=90:color=0x171717@1:t=fill,drawbox=x=488:y=918:w=90:h=90:color=0x171717@1:t=fill" \
  -frames:v 1 "$PUBLIC_DIR/input/wheel-detail.png" >/dev/null 2>&1
ffmpeg -y -f lavfi -i color=c=0xf4f4f5:s=900x1200 \
  -vf "drawbox=x=180:y=560:w=540:h=360:color=0x9ca3af@1:t=fill,drawbox=x=220:y=610:w=220:h=260:color=0xe5e7eb@1:t=fill,drawbox=x=470:y=610:w=210:h=260:color=0xd1d5db@1:t=fill" \
  -frames:v 1 "$PUBLIC_DIR/input/open-storage.png" >/dev/null 2>&1
ffmpeg -y -f lavfi -i sine=frequency=620:duration=12 -ac 2 -ar 48000 "$PUBLIC_DIR/input/voiceover.mp3" >/dev/null 2>&1
ffmpeg -y -f lavfi -i sine=frequency=220:duration=12 -ac 2 -ar 48000 "$PUBLIC_DIR/input/bgm.mp3" >/dev/null 2>&1

cat >"$TMP_DIR/timeline-plan.json" <<'JSON'
{
  "schema": "clipanvil.remotion_timeline.v1",
  "composition": "MarketingTimeline",
  "output": {"width": 1080, "height": 1920, "fps": 30, "duration_sec": 12, "codec": "h264", "audio_codec": "aac"},
  "theme": {"brand_colors": ["#111827", "#F5C542"], "font_family": "Noto Sans CJK SC", "style": "mixed_cost_product_ad"},
  "segments": [
    {
      "id": "seg_hero_video",
      "shot_ref": "shot_hero",
      "start_sec": 0,
      "end_sec": 5,
      "layout": "hero_packshot",
      "visual_focus": "Seedance hero",
      "assets": [{"role": "primary", "type": "video", "workspace_path": "/workspace/input/hero-seedance.mp4"}],
      "motion": {"preset": "push_in", "intensity": 0.45},
      "transition_in": {"type": "cut", "duration_sec": 0},
      "caption": {"source": "audio_cue", "text": "悦行行李箱，短途轻松出发。", "start_sec": 0, "end_sec": 5, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_wheels_still",
      "shot_ref": "shot_wheels",
      "start_sec": 5,
      "end_sec": 8.5,
      "layout": "detail_focus",
      "visual_focus": "顺滑万向轮",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/wheel-detail.png"}],
      "motion": {"preset": "spotlight_reveal", "intensity": 0.55},
      "transition_in": {"type": "slide", "duration_sec": 0.25},
      "caption": {"source": "audio_cue", "text": "顺滑万向轮，转弯更从容。", "start_sec": 5, "end_sec": 8.5, "position": "subtitle_bottom"}
    },
    {
      "id": "seg_storage_still",
      "shot_ref": "shot_storage",
      "start_sec": 8.5,
      "end_sec": 12,
      "layout": "open_storage",
      "visual_focus": "分区收纳",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/open-storage.png"}],
      "motion": {"preset": "pull_out", "intensity": 0.55},
      "transition_in": {"type": "wipe", "duration_sec": 0.25},
      "caption": {"source": "audio_cue", "text": "打开一看，分区清楚。", "start_sec": 8.5, "end_sec": 12, "position": "subtitle_bottom"}
    }
  ],
  "audio_tracks": [
    {"id": "voiceover_main", "role": "voiceover", "workspace_path": "/workspace/input/voiceover.mp3", "start_sec": 0, "volume": 1},
    {"id": "bgm_main", "role": "bgm", "workspace_path": "/workspace/input/bgm.mp3", "start_sec": 0, "volume": 0.22, "loop": true}
  ],
  "captions": {"source": "audio_cue", "single_lane": true, "max_chars_per_line": 18, "style": "commerce_subtitle"}
}
JSON

OUT="$PUBLIC_DIR/output/final-remotion-mixed-media.mp4"
node "$PROJECT_DIR/src/render.mjs" --props "$TMP_DIR/timeline-plan.json" --out "$OUT" --public-dir "$PUBLIC_DIR"

video_dims="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$OUT")"
audio_stream="$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_type -of csv=p=0 "$OUT")"
echo "ffprobe video_dims=$video_dims audio_stream=$audio_stream"
grep -Eq '^1080,1920,?$' <<<"$video_dims"
grep -q '^audio$' <<<"$audio_stream"

duration="$(ffprobe -v error -show_entries format=duration -of default=nk=1:nw=1 "$OUT")"
awk -v duration="$duration" 'BEGIN { if (duration < 11.5 || duration > 12.8) exit 1 }'

echo "M13.4 Remotion mixed media smoke passed: $OUT duration=$duration"
