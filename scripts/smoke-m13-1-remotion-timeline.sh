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

ffmpeg -y -f lavfi -i color=c=0xd8dde7:s=900x1200 -vf "drawbox=x=260:y=430:w=380:h=520:color=0x626b78@1:t=fill,drawbox=x=305:y=470:w=290:h=430:color=0xc4cad3@1:t=fill" -frames:v 1 "$PUBLIC_DIR/input/product.png" >/dev/null 2>&1
ffmpeg -y -f lavfi -i sine=frequency=620:duration=10 -ac 2 -ar 48000 "$PUBLIC_DIR/input/voiceover.mp3" >/dev/null 2>&1
ffmpeg -y -f lavfi -i sine=frequency=220:duration=10 -ac 2 -ar 48000 "$PUBLIC_DIR/input/bgm.mp3" >/dev/null 2>&1

cat >"$TMP_DIR/timeline-plan.json" <<'JSON'
{
  "schema": "clipanvil.remotion_timeline.v1",
  "composition": "MarketingTimeline",
  "output": {"width": 1080, "height": 1920, "fps": 30, "duration_sec": 10, "codec": "h264", "audio_codec": "aac"},
  "theme": {"brand_colors": ["#111827", "#F5C542"], "font_family": "Noto Sans CJK SC", "style": "premium_product_ad"},
  "segments": [
    {
      "id": "seg_hero",
      "shot_ref": "shot_01",
      "start_sec": 0,
      "end_sec": 10,
      "layout": "hero_packshot",
      "assets": [{"role": "primary", "type": "image", "workspace_path": "/workspace/input/product.png"}],
      "motion": {"preset": "push_in", "intensity": "medium"},
      "text_layers": [{"role": "hook", "text": "Travel Light", "start_sec": 0.4, "end_sec": 4.2, "position": "upper_third", "animation": "kinetic_rise"}],
      "caption": {"source": "audio_cue", "text": "Smooth wheels, easy weekend trips.", "start_sec": 0, "end_sec": 10, "position": "subtitle_bottom"}
    }
  ],
  "audio_tracks": [
    {"id": "voiceover_main", "role": "voiceover", "workspace_path": "/workspace/input/voiceover.mp3", "start_sec": 0, "volume": 1},
    {"id": "bgm_main", "role": "bgm", "workspace_path": "/workspace/input/bgm.mp3", "start_sec": 0, "volume": 0.22, "loop": true}
  ],
  "captions": {"source": "audio_plan.cue_plan", "single_lane": true, "max_chars_per_line": 18, "style": "commerce_subtitle"}
}
JSON

OUT="$PUBLIC_DIR/output/final-remotion-timeline.mp4"
node "$PROJECT_DIR/src/render.mjs" --props "$TMP_DIR/timeline-plan.json" --out "$OUT" --public-dir "$PUBLIC_DIR"

video_dims="$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$OUT")"
audio_stream="$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_type -of csv=p=0 "$OUT")"
echo "ffprobe video_dims=$video_dims audio_stream=$audio_stream"
grep -Eq '^1080,1920,?$' <<<"$video_dims"
grep -q '^audio$' <<<"$audio_stream"

duration="$(ffprobe -v error -show_entries format=duration -of default=nk=1:nw=1 "$OUT")"
awk -v duration="$duration" 'BEGIN { if (duration < 9.5 || duration > 10.7) exit 1 }'

echo "M13.1 Remotion timeline smoke passed: $OUT duration=$duration"
