#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "== M8 skill runtime focused E2E =="
(
  cd apps/server
  GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'M8SkillRuntimeEndToEnd|LoadAgentSkill|LoadAgentSkillResource' -count=1
)

echo "== M8 skill quality loop smoke =="
bash -n scripts/smoke-m8-3-skill-quality-loop.sh
./scripts/smoke-m8-3-skill-quality-loop.sh

echo "== M8 server regression =="
GOCACHE=/private/tmp/clipanvil-go-build make server-test

echo "== M8 diff check =="
git diff --check

echo "M8 skill runtime E2E PASS"
