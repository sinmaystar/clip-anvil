#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/verify-template-video-quality.sh --video FILE --product-image FILE [--expect-audio]

Checks that a generated template video has a video stream, optionally has an
audio stream, and contains non-empty product-region pixels in a middle frame.
USAGE
}

video=""
product_image=""
expect_audio=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --video)
      video="${2:-}"
      shift 2
      ;;
    --product-image)
      product_image="${2:-}"
      shift 2
      ;;
    --expect-audio)
      expect_audio=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$video" || ! -f "$video" ]]; then
  echo "--video must point to an existing file" >&2
  exit 1
fi
if [[ -z "$product_image" || ! -f "$product_image" ]]; then
  echo "--product-image must point to an existing file" >&2
  exit 1
fi
if ! command -v ffprobe >/dev/null 2>&1; then
  echo "ffprobe is required" >&2
  exit 1
fi
if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "ffmpeg is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

video_json="$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_type,codec_name,width,height,duration -of json "$video")"
video_codec="$(jq -r '.streams[0].codec_name // empty' <<<"$video_json")"
if [[ -z "$video_codec" ]]; then
  echo "video stream missing" >&2
  echo "$video_json" >&2
  exit 1
fi

audio_json="$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_type,codec_name,duration -of json "$video")"
audio_codec="$(jq -r '.streams[0].codec_name // empty' <<<"$audio_json")"
if [[ "$expect_audio" == "1" && -z "$audio_codec" ]]; then
  echo "audio stream missing" >&2
  echo "$audio_json" >&2
  exit 1
fi

python3 - "$video" "$product_image" <<'PY'
import math
import subprocess
import sys

video = sys.argv[1]
product = sys.argv[2]

def raw_rgb_from_ffmpeg(args, width, height):
    proc = subprocess.run(
        ["ffmpeg", "-hide_banner", "-loglevel", "error", *args, "-f", "rawvideo", "-pix_fmt", "rgb24", "-"],
        check=True,
        stdout=subprocess.PIPE,
    )
    expected = width * height * 3
    if len(proc.stdout) != expected:
        raise SystemExit(f"unexpected raw frame size: {len(proc.stdout)} != {expected}")
    return proc.stdout

width = 240
height = 320
frame = raw_rgb_from_ffmpeg([
    "-ss", "3",
    "-i", video,
    "-frames:v", "1",
    "-vf", f"crop=iw*0.76:ih*0.50:iw*0.12:ih*0.10,scale={width}:{height}",
], width, height)

product_frame = raw_rgb_from_ffmpeg([
    "-i", product,
    "-frames:v", "1",
    "-vf", f"scale={width}:{height}:force_original_aspect_ratio=decrease,pad={width}:{height}:(ow-iw)/2:(oh-ih)/2:color=black",
], width, height)

def luma(buf, index):
    r = buf[index]
    g = buf[index + 1]
    b = buf[index + 2]
    return 0.2126 * r + 0.7152 * g + 0.0722 * b

values = [luma(frame, i) for i in range(0, len(frame), 3)]
mean = sum(values) / len(values)
variance = sum((v - mean) ** 2 for v in values) / len(values)

edge = 0.0
count = 0
for y in range(height - 1):
    for x in range(width - 1):
        i = (y * width + x) * 3
        right = i + 3
        down = ((y + 1) * width + x) * 3
        edge += abs(luma(frame, i) - luma(frame, right)) + abs(luma(frame, i) - luma(frame, down))
        count += 2
edge /= max(count, 1)

product_values = [luma(product_frame, i) for i in range(0, len(product_frame), 3)]
product_mean = sum(product_values) / len(product_values)
product_variance = sum((v - product_mean) ** 2 for v in product_values) / len(product_values)

if variance < 120 or edge < 2.2:
    raise SystemExit(f"product region appears blank or broken: variance={variance:.2f} edge={edge:.2f}")
if product_variance < 40:
    raise SystemExit(f"product image appears too blank for verification: variance={product_variance:.2f}")

print(f"product_region_variance={variance:.2f}")
print(f"product_region_edge={edge:.2f}")
print(f"product_reference_variance={product_variance:.2f}")
PY

echo "template video quality check passed"
echo "video_codec=$video_codec"
if [[ -n "$audio_codec" ]]; then
  echo "audio_codec=$audio_codec"
else
  echo "audio_codec=none"
fi
