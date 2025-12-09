#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

u=${1:?missing username}

git config --global codespaces-theme.hide-status 1
sudo mkdir -p /go/pkg
sudo chown vscode:golang /go/pkg
sudo mkdir -p "/home/${u}/.cache/go-build"
sudo chown vscode:vscode "/home/${u}/.cache/go-build"

bashrc="/home/${u}/.bashrc"
aliases_file="/home/${u}/.aliases"

cat <<EOF > "$aliases_file"
alias ll='ls -l --color=auto'
EOF

echo ". \"${aliases_file}\"" >> "$bashrc"

cat <<'GIT' >> "$bashrc"
source /usr/share/bash-completion/completions/git
source /etc/bash_completion.d/git-prompt
GIT_PS1_SHOWDIRTYSTATE=true
GIT

cat <<'PROMPT' >> "$bashrc"
export PS1='$(export XIT=$? && echo -n "\[\033[0;32m\]\u " && [ "$XIT" -ne "0" ] && echo -n "\[\033[1;31m\]➜" || echo -n "\[\033[0m\]➜" \
) \[\033[1;34m\]\w\[\033[31m\]$(__git_ps1)\[\033[00m\]\$ '
export PROMPT_DIRTRIM=2
PROMPT

# Go ツールのエイリアスを bashrc に追加する
gotoolalias() {
    local tool=$1
    local cmd="alias $tool='CGO_ENABLED=0 go tool $tool'"
    echo "$cmd" | tee -a "$bashrc"
}

gotoolalias cobra-cli
. "$bashrc"

CGO_ENABLED=0 go tool gocomplete -install -y
echo '. <(cobra-cli completion bash || true)' >> "$bashrc"

LSCRIPT="$DIR/postCreateCommand.local.sh"
if [ -x "$LSCRIPT" ]; then
    "$LSCRIPT" "$u"
else
    echo "$LSCRIPT is not executable"
fi

go version
