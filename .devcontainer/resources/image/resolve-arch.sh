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

ARCH=${GO_ARCH:-}
TARGET=${TARGETARCH:-}
if [ -z "${ARCH}" ] || [ "${ARCH}" = "auto" ]; then
    if [ -z "${TARGET}" ]; then
        echo "GO_ARCH=auto requires TARGETARCH or an explicit GO_ARCH value. Supported: amd64, arm64 (aliases: x86_64, aarch64)." >&2
        exit 1
    fi
    ARCH=$(normalize_arch "${TARGET}") || fail_unsupported TARGETARCH "${TARGET}"
else
    ARCH=$(normalize_arch "${ARCH}") || fail_unsupported GO_ARCH "${ARCH}"
fi

RG_ARCH=$(resolve_rg_arch "${ARCH}") || fail_unsupported ARCH "${ARCH}"

MODE=${1:-go}
if [ "${MODE}" = "rg" ]; then
    printf '%s\n' "${RG_ARCH}"
else
    printf '%s\n' "${ARCH}"
fi
