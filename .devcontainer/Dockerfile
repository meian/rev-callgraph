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
COPY .devcontainer/resources/image/resolve-arch.sh /usr/local/bin/resolve-arch.sh
RUN ARCH=$(/usr/local/bin/resolve-arch.sh); \
    GO_TARBALL=go${GO_VERSION}.linux-${ARCH}.tar.gz; \
    curl -LO https://golang.org/dl/${GO_TARBALL}; \
    tar -C /tmp -xzf ${GO_TARBALL}

# ツール類の取得
FROM alpine:3.22.0 AS tool-builder
ARG GO_ARCH
ARG TARGETARCH
ARG RG_VERSION

RUN apk add --no-cache curl tar jq
RUN mkdir -p /tmp/ulb
COPY --from=latest-go-fetcher /usr/local/bin/resolve-arch.sh /usr/local/bin/resolve-arch.sh
COPY .devcontainer/resources/image/fetch-ripgrep.sh /usr/local/bin/fetch-ripgrep.sh
RUN set -eux; \
    RG_ARCH=$(/usr/local/bin/resolve-arch.sh rg); \
    /usr/local/bin/fetch-ripgrep.sh "${RG_VERSION}" "${RG_ARCH}" /tmp/ulb

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
