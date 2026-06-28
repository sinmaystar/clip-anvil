#!/usr/bin/env bash
set -euo pipefail

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "ffmpeg is required for this smoke script." >&2
  exit 1
fi

if ! command -v ffprobe >/dev/null 2>&1; then
  echo "ffprobe is required for this smoke script." >&2
  exit 1
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/clipanvil-m7-3-audio.XXXXXX")"
if [[ "${KEEP_M7_3_SMOKE_OUTPUT:-}" != "1" ]]; then
  trap 'rm -rf "$tmp_dir"' EXIT
fi

shot_1="$tmp_dir/shot-01.mp4"
shot_2="$tmp_dir/shot-02.mp4"
voiceover="$tmp_dir/voiceover.wav"
bgm="$tmp_dir/bgm.wav"
output="$tmp_dir/final-audio.mp4"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "color=c=#3467eb:s=320x180:d=2:r=24" \
  -vf "format=yuv420p" \
  "$shot_1"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "color=c=#f59e0b:s=320x180:d=2:r=24" \
  -vf "format=yuv420p" \
  "$shot_2"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "sine=frequency=880:duration=4" \
  -c:a pcm_s16le \
  "$voiceover"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "sine=frequency=220:duration=4.5" \
  -c:a pcm_s16le \
  "$bgm"

filter="[0:v][1:v]concat=n=2:v=1:a=0[vcat];"
filter+="[vcat]format=yuv420p,setsar=1[vout];"
filter+="[2:a]atrim=start=0:duration=4.000,asetpts=PTS-STARTPTS,volume=1.000,afade=t=in:st=0:d=0.050,afade=t=out:st=3.900:d=0.100[a0raw];"
filter+="[a0raw]asplit=2[a0][a0side];"
filter+="[3:a]atrim=start=0:duration=4.000,asetpts=PTS-STARTPTS,volume=0.280,afade=t=in:st=0:d=0.500,afade=t=out:st=3.000:d=1.000[a1];"
filter+="[a1][a0side]sidechaincompress=threshold=0.080:ratio=8.000:attack=20:release=250[a1duck];"
filter+="[a0][a1duck]amix=inputs=2:duration=shortest:dropout_transition=0[aout]"

ffmpeg -hide_banner -loglevel error -y \
  -i "$shot_1" \
  -i "$shot_2" \
  -i "$voiceover" \
  -stream_loop -1 -i "$bgm" \
  -filter_complex "$filter" \
  -map "[vout]" \
  -map "[aout]" \
  -c:v libx264 \
  -preset fast \
  -crf 28 \
  -c:a aac \
  -b:a 128k \
  -shortest \
  "$output"

audio_codec="$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_name -of default=noprint_wrappers=1:nokey=1 "$output")"
if [[ "$audio_codec" != "aac" ]]; then
  echo "expected final audio codec aac, got ${audio_codec:-<none>}" >&2
  exit 1
fi

video_codec="$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of default=noprint_wrappers=1:nokey=1 "$output")"
if [[ -z "$video_codec" ]]; then
  echo "expected final video stream." >&2
  exit 1
fi

echo "m7.3 audio composer smoke passed"
echo "output=$output"
echo "audio_codec=$audio_codec"
echo "video_codec=$video_codec"
