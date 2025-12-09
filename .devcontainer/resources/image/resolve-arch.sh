#!/bin/sh
set -e

normalize_arch() {
    case "$1" in
        amd64|x86_64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) return 1 ;;
    esac
}

fail_unsupported() {
    echo "Unsupported $1: $2. Supported: amd64, arm64 (aliases: x86_64, aarch64)." >&2
    exit 1
}

resolve_rg_arch() {
    case "$1" in
        amd64) echo "x86_64" ;;
        arm64) echo "aarch64" ;;
        *) return 1 ;;
    esac
}

TARGET=${TARGETARCH:-}
if [ -z "${TARGET}" ]; then
    TARGET=$(uname -m)
fi
ARCH=$(normalize_arch "${TARGET}") || fail_unsupported TARGETARCH "${TARGET}"

RG_ARCH=$(resolve_rg_arch "${ARCH}") || fail_unsupported ARCH "${ARCH}"

MODE=${1:?mode required}
if [ "${MODE}" = "rg" ]; then
    printf '%s\n' "${RG_ARCH}"
elif [ "${MODE}" = "go" ]; then
    printf '%s\n' "${ARCH}"
else
    fail_unsupported MODE "${MODE}"
fi
