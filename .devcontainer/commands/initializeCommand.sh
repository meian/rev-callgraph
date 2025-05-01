#!/bin/bash -e

# go.modからGoバージョンを抽出
GO_VERSION=$(grep -m1 '^go ' ./go.mod | awk '{print $2}')

# DockerfileのARG GO_VERSION行を書き換え
sed -i "s/^ARG GO_VERSION=.*$/ARG GO_VERSION=${GO_VERSION}/" ./.devcontainer/Dockerfile
echo "Updated Dockerfile with Go version ${GO_VERSION}"
