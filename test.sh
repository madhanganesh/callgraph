#!/usr/bin/env bash
# Run callgraph integration tests. Pass a language (go|python|rust) to scope.
set -euo pipefail

cd "$(dirname "$0")"

case "${1:-all}" in
  go)     pkgs=(./test/go/...) ;;
  python) pkgs=(./test/python/...) ;;
  rust)   pkgs=(./test/rust/...) ;;
  all)    pkgs=(./test/go/... ./test/python/... ./test/rust/...) ;;
  *)      echo "usage: $0 [go|python|rust|all]" >&2; exit 2 ;;
esac

exec go test -v -timeout 600s "${pkgs[@]}"
