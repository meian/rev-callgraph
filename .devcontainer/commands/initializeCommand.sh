#!/bin/bash -e

# go.modからGoバージョンを抽出
GO_VERSION=$(grep -m1 '^go ' ./go.mod | awk '{print $2}')

# DockerfileのARG GO_VERSION行を書き換え
sed -i "s/^ARG GO_VERSION=.*$/ARG GO_VERSION=${GO_VERSION}/" ./.devcontainer/Dockerfile
echo "Updated Dockerfile with Go version ${GO_VERSION}"

env='./.devcontainer/.env.devcontainer'
> "$env"

# GitHub CLI用の認証情報を取得
if which gh &> /dev/null; then
    GH_TOKEN=$(gh auth token)
    if [ -z "$GH_TOKEN" ]; then
        echo "Skip authentication because gh auth token is empty."
    else
        echo "GH_TOKEN=${GH_TOKEN}" > "$env"
    fi
else
    echo "GitHub CLI is not installed. Skipping authentication."
    sed -i '/^GH_TOKEN=/d' "$env"
fi
