ARG GO_VERSION=1.24.1

# ビルド変数で指定したバージョンのgolangを取得
FROM alpine:latest AS latest-go-fetcher
ARG GO_VERSION
ENV GO_VERSION=${GO_VERSION}

RUN apk add --no-cache curl tar
RUN curl -LO https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz
RUN tar -C /tmp -xzf go${GO_VERSION}.linux-amd64.tar.gz

# ツール類の取得
FROM golang:${GO_VERSION} AS tool-builder

RUN mkdir -p /tmp/ulb
RUN curl -LO https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz
RUN tar zxf ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz 
RUN mv ripgrep-14.1.0-x86_64-unknown-linux-musl/rg /tmp/ulb

# vscodeのGo拡張用ツールの取得
FROM golang:${GO_VERSION} AS golsp-builder

RUN go install golang.org/x/tools/gopls@v0.18.1
RUN go install github.com/cweill/gotests/gotests@v1.6.0
RUN go install github.com/go-delve/delve/cmd/dlv@v1.24.1

# devcontainerのイメージ
FROM mcr.microsoft.com/devcontainers/go:1.23-bookworm

ARG USERNAME

RUN apt-get update \
    && apt-get install -y \
    vim \
    curl \
    sqlite3 \
    && apt-get autoremove -y && apt-get clean -y && rm -rf /var/lib/apt/lists/* /tmp/library-scripts/ \
    && ln -sf /usr/share/zoneinfo/Asia/Tokyo /etc/localtime

COPY --from=tool-builder /tmp/ulb /usr/local/bin
COPY --from=tool-builder /go/bin /go/bin

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
