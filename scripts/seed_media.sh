#!/usr/bin/env bash
# Generate the short clips and cover frames that `make seed` points at.
#
# The clips are rendered locally with ffmpeg's `gradients` source, so there is no third-party
# footage, no licence question and no network download: every byte is produced by this script.
# Vertical 540x960, H.264 + faststart (moov atom up front, required for browser playback),
# a few hundred KB each.
#
# Usage: bash scripts/seed_media.sh [ffmpeg_path]
#   FORCE=1 bash scripts/seed_media.sh   # re-render even if the files already exist

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VIDEO_DIR="$ROOT/uploads/videos/seed"
COVER_DIR="$ROOT/uploads/covers/seed"

FFMPEG="${1:-}"
if [[ -z "$FFMPEG" ]]; then
  if command -v ffmpeg >/dev/null 2>&1; then
    FFMPEG="$(command -v ffmpeg)"
  elif command -v python3 >/dev/null 2>&1 && python3 -c "import imageio_ffmpeg" >/dev/null 2>&1; then
    FFMPEG="$(python3 -c "import imageio_ffmpeg; print(imageio_ffmpeg.get_ffmpeg_exe())")"
  fi
fi

if [[ -z "$FFMPEG" || ! -x "$FFMPEG" ]]; then
  echo "ffmpeg not found. Install one of:" >&2
  echo "  sudo apt install ffmpeg" >&2
  echo "  pip install imageio-ffmpeg" >&2
  echo "  bash scripts/seed_media.sh /path/to/ffmpeg" >&2
  exit 1
fi

echo "Using ffmpeg: $FFMPEG"
mkdir -p "$VIDEO_DIR" "$COVER_DIR"

# name | seconds | start colour | end colour | seed
CLIPS=(
  "travel-01|8|0xFF9A5A|0xFF5F6D|11"
  "travel-02|10|0x134E5E|0x71B280|12"
  "travel-03|9|0x2193B0|0x6DD5ED|13"
  "travel-04|11|0x2C3E50|0x4CA1AF|14"
  "travel-05|7|0x5F2C82|0x49A09D|15"
  "travel-06|12|0x355C7D|0xC06C84|16"
  "food-01|9|0xF7971E|0xFFD200|21"
  "food-02|10|0xFF512F|0xDD2476|22"
  "city-01|8|0x232526|0x414345|31"
  "city-02|11|0x1F1C2C|0x928DAB|32"
  "pet-01|6|0xF2994A|0xF2C94C|41"
  "pet-02|7|0x56AB2F|0xA8E063|42"
)

for entry in "${CLIPS[@]}"; do
  IFS='|' read -r name seconds color0 color1 seed <<<"$entry"
  video="$VIDEO_DIR/$name.mp4"
  cover="$COVER_DIR/$name.jpg"

  if [[ -f "$video" && -f "$cover" && "${FORCE:-0}" != "1" ]]; then
    echo "skip $name (already exists)"
    continue
  fi

  "$FFMPEG" -hide_banner -loglevel error -y \
    -f lavfi -i "gradients=s=540x960:d=$seconds:c0=$color0:c1=$color1:speed=0.05:seed=$seed" \
    -an -c:v libx264 -preset veryfast -crf 30 -pix_fmt yuv420p \
    -movflags +faststart "$video"

  # A frame from the same clip keeps the cover honest: what you see in the grid is what plays.
  "$FFMPEG" -hide_banner -loglevel error -y \
    -ss 1 -i "$video" -frames:v 1 -q:v 3 "$cover"

  size=$(stat -c%s "$video" 2>/dev/null || stat -f%z "$video")
  echo "rendered $name.mp4 (${seconds}s, $((size / 1024)) KB) + cover"
done

echo
echo "Media ready in $VIDEO_DIR and $COVER_DIR"
