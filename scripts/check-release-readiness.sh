#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
export GOWORK=off

module=github.com/githubixx/chroma-ui
if ! dependency="$(go list -mod=readonly -m -f '{{.Version}}{{if .Replace}} replaced{{end}}' "$module")"; then
    echo "Release blocked: pin a real published Chroma UI revision after code review; local workspace verification is not a standalone release check." >&2
    exit 1
fi
case "$dependency" in
    ""|*" replaced")
        echo "Release blocked: Chroma UI must have a versioned dependency without a replace directive." >&2
        exit 1
        ;;
esac

if ! go mod tidy -diff; then
    echo "Release blocked: go.mod/go.sum are not tidy or dependencies could not be resolved. Review the diagnostics; update manifests deliberately after pinning the published revision." >&2
    exit 1
fi
go mod verify
printf 'Standalone dependency preflight passed: %s %s\n' "$module" "$dependency"
