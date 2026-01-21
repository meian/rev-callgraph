#!/bin/sh
set -e

VERSION=${1:?version required}
ARCH=${2:?arch required}
DEST_DIR=${3:-/tmp/ulb}

RG_RELEASE_URL="https://api.github.com/repos/BurntSushi/ripgrep/releases/tags/${VERSION}"
RG_PREFIX="ripgrep-${VERSION}-${ARCH}-unknown-linux-"

if ! RG_JSON=$(curl --fail -sSL "${RG_RELEASE_URL}"); then
    echo "Failed to fetch ripgrep release metadata: ${RG_RELEASE_URL}" >&2
    exit 1
fi

RG_TARBALL=$(printf '%s' "${RG_JSON}" | jq -r --arg prefix "${RG_PREFIX}" '[.assets[].name | select(startswith($prefix) and endswith(".tar.gz"))] | sort | .[0] // ""')
if [ -z "${RG_TARBALL}" ]; then
    echo "ripgrep tarball not found for ${VERSION} arch=${ARCH}" >&2
    exit 1
fi

if ! curl --fail -LO "https://github.com/BurntSushi/ripgrep/releases/download/${VERSION}/${RG_TARBALL}"; then
    echo "Failed to download ripgrep tarball ${RG_TARBALL}" >&2
    exit 1
fi

tar zxf "${RG_TARBALL}"
RG_DIR="${RG_TARBALL%.tar.gz}"
if [ ! -x "${RG_DIR}/rg" ]; then
    echo "ripgrep binary not found at ${RG_DIR}/rg" >&2
    exit 1
fi

install -d -m 0755 "${DEST_DIR}"
install -m 0755 "${RG_DIR}/rg" "${DEST_DIR}/rg"
