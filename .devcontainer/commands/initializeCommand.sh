#!/usr/bin/env bash
set -euo pipefail

# go.mod から Go バージョンを抽出
GO_VERSION=$(awk '/^go / { print $2; exit }' go.mod)
if [[ -z "${GO_VERSION}" ]]; then
    echo "Go version not found in go.mod" >&2
    exit 1
fi

# BSD/GNU どちらでも動くように in-place sed をラップ
inplace_sed() {
    local pattern="$1"
    local file="$2"
    if sed --version >/dev/null 2>&1; then
        sed -i "${pattern}" "${file}"
    else
        sed -i '' "${pattern}" "${file}"
    fi
}

inplace_sed "s/^ARG GO_VERSION=.*/ARG GO_VERSION=${GO_VERSION}/" .devcontainer/Dockerfile
echo "Updated Dockerfile with Go version ${GO_VERSION}"

env_path='.devcontainer/tmp/.env.devcontainer'
: > "${env_path}"

# GitHub CLI 用の認証情報を取得
if command -v gh >/dev/null 2>&1; then
    GH_TOKEN=$(gh auth token)
    if [[ -z "${GH_TOKEN}" ]]; then
        echo "Skip authentication because gh auth token is empty."
    else
        echo "GH_TOKEN=${GH_TOKEN}" > "${env_path}"
    fi
else
    echo "GitHub CLI is not installed. Skipping authentication."
fi
