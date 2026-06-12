#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

export GOCACHE="$repo_root/.cache/go-build"
export GOLANGCI_LINT_CACHE="$repo_root/.cache/golangci-lint"
mkdir -p "$GOCACHE" "$GOLANGCI_LINT_CACHE"

changed_files=()
while IFS= read -r -d '' entry; do
  status="${entry:0:2}"
  path="${entry:3}"

  if [[ "$status" == R* || "$status" == C* ]]; then
    IFS= read -r -d '' path
  fi

  if [[ "$status" == D* || "$status" == *D ]]; then
    continue
  fi

  if [[ -f "$path" ]]; then
    changed_files+=("$path")
  fi
done < <(git status --porcelain=v1 -z --untracked-files=all)

go_files=()
ts_files=()
for file in "${changed_files[@]}"; do
  case "$file" in
    node_modules/*|.pnpm-store/*|.cache/*|dist/*|apps/web/tsconfig.tsbuildinfo)
      ;;
    apps/server/*.go|apps/server/**/*.go)
      go_files+=("$file")
      ;;
    *.ts|*.tsx)
      ts_files+=("$file")
      ;;
  esac
done

if ((${#go_files[@]})); then
  gofmt -w "${go_files[@]}"
  (cd apps/server && golangci-lint run --fix ./...)
fi

if ((${#ts_files[@]})); then
  eslint_bin="./node_modules/.bin/eslint"
  if [[ -x "$eslint_bin" ]]; then
    "$eslint_bin" --fix "${ts_files[@]}"
  else
    pnpm eslint --fix "${ts_files[@]}"
  fi
fi
