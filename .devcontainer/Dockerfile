ARG GO_VERSION=1.24.4
ARG GO_ARCH=auto
ARG RG_VERSION=14.1.0

# ビルド変数で指定したバージョンのgolangを取得
FROM alpine:3.22.0 AS latest-go-fetcher
ARG GO_VERSION
ARG GO_ARCH
# TARGETARCH は buildx の --platform 指定時に自動で設定される
ARG TARGETARCH
ENV GO_VERSION=${GO_VERSION}

RUN apk add --no-cache curl tar
RUN cat <<'EOF' >/usr/local/bin/resolve-arch.sh
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
EOF
RUN chmod +x /usr/local/bin/resolve-arch.sh
RUN ARCH=$(/usr/local/bin/resolve-arch.sh); \
    GO_TARBALL=go${GO_VERSION}.linux-${ARCH}.tar.gz; \
    curl -LO https://golang.org/dl/${GO_TARBALL}; \
    tar -C /tmp -xzf ${GO_TARBALL}

# ツール類の取得
FROM alpine:3.22.0 AS tool-builder
ARG GO_ARCH
ARG TARGETARCH
ARG RG_VERSION

RUN apk add --no-cache curl tar
RUN mkdir -p /tmp/ulb
COPY --from=latest-go-fetcher /usr/local/bin/resolve-arch.sh /usr/local/bin/resolve-arch.sh
RUN set -eux; \
    RG_ARCH=$(/usr/local/bin/resolve-arch.sh rg); \
    RG_TARBALL="ripgrep-${RG_VERSION}-${RG_ARCH}-unknown-linux-musl.tar.gz"; \
    curl -LO https://github.com/BurntSushi/ripgrep/releases/download/${RG_VERSION}/${RG_TARBALL}; \
    tar zxf ${RG_TARBALL}; \
    RG_DIR="ripgrep-${RG_VERSION}-${RG_ARCH}-unknown-linux-musl"; \
    if [ ! -x "${RG_DIR}/rg" ]; then \
        echo "ripgrep binary not found at ${RG_DIR}/rg" >&2; \
        exit 1; \
    fi; \
    mv "${RG_DIR}/rg" /tmp/ulb

# vscodeのGo拡張用ツールの取得
FROM golang:${GO_VERSION} AS golsp-builder

RUN go install golang.org/x/tools/gopls@v0.18.1
RUN go install github.com/cweill/gotests/gotests@v1.6.0
RUN go install github.com/go-delve/delve/cmd/dlv@v1.24.1

# devcontainerのイメージ
FROM mcr.microsoft.com/devcontainers/go:1.24-bookworm

ARG USERNAME

RUN apt-get update \
    && apt-get install -y \
    vim \
    curl \
    sqlite3 \
    && apt-get autoremove -y && apt-get clean -y && rm -rf /var/lib/apt/lists/* /tmp/library-scripts/ \
    && ln -sf /usr/share/zoneinfo/Asia/Tokyo /etc/localtime

COPY --from=tool-builder /tmp/ulb /usr/local/bin

COPY --from=latest-go-fetcher /tmp/go /tmp/go
RUN GOPATH=$(dirname $(dirname $(which go))) \
    && rm ${GOPATH} -rf \
    && mv /tmp/go ${GOPATH}

RUN curl -sL install.mob.sh | sudo sh

RUN if [ -z "${USERNAME}" ] ; then exit 1 ; fi \
    && useradd -u1000 -g1000 -s/bin/bash -b/home/${USERNAME} ${USERNAME} 2> /dev/null || true

RUN install -d -m 0755 -o ${USERNAME} -g ${USERNAME} /workspaces

USER ${USERNAME}

COPY --from=golsp-builder --chown=${USERNAME}:golang /go/bin /go/bin

COPY --chown=${USERNAME}:${USERNAME} resources/.vimrc /home/${USERNAME}

RUN mkdir -p /home/${USERNAME}/.bin \
    && echo 'export PATH=$PATH:/home/'${USERNAME}'/.bin' >> /home/${USERNAME}/.bashrc

COPY --chown=${USERNAME}:${USERNAME} resources/hub /home/${USERNAME}/.bin
