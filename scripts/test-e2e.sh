#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p test/e2e/snapshots test-results playwright-report
build_args=()
if [[ -n "${CHROMA_UI_DIR:-}" ]]; then
  build_args+=(--build-context "chroma-ui=$CHROMA_UI_DIR" --build-arg FIXTURE_BUILDER=fixture-builder-workspace)
fi
docker build --platform linux/amd64 "${build_args[@]}" -f test/e2e/Dockerfile -t vdradmin-go-ui-tests .
docker run --platform linux/amd64 --rm --init --shm-size=1g \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e CI=true \
  -v "$PWD/test/e2e/snapshots:/app/test/e2e/snapshots" \
  -v "$PWD/test-results:/app/test-results" \
  -v "$PWD/playwright-report:/app/playwright-report" \
  vdradmin-go-ui-tests npm run test:e2e -- "$@"
