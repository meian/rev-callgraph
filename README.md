# rev-callgraph

Go の関数やメソッドを起点に、ワークスペース内の呼び出し元を逆向きにたどる CLI です。複数 module を含むディレクトリを解析でき、同じ major 系列なら依存 version の minor・patch が異なっていても候補に含めます。

## インストール

### 配布バイナリを使う

[GitHub Releases](https://github.com/meian/rev-callgraph/releases) で使いたいバージョンを開き、次の表から選んだファイルと `SHA256SUMS` を同じディレクトリにダウンロードします。

| OS | CPU | ファイル |
| --- | --- | --- |
| Linux | x86-64 | `rev-callgraph-linux-amd64` |
| Linux | ARM64 | `rev-callgraph-linux-arm64` |
| macOS | Intel | `rev-callgraph-darwin-amd64` |
| macOS | Apple Silicon | `rev-callgraph-darwin-arm64` |
| Windows | x64 | `rev-callgraph-windows-amd64.zip` |

ダウンロード先で次のコマンドを実行し、表示された SHA-256 が `SHA256SUMS` の同じファイル名の値と一致することを確認します。
`<ファイル名>` は表から選んだ名前に置き換えます。

| OS | SHA-256 の確認コマンド |
| --- | --- |
| Linux | `sha256sum <ファイル名>` |
| macOS | `shasum -a 256 <ファイル名>` |
| Windows（PowerShell） | `Get-FileHash <ファイル名> -Algorithm SHA256` |

Linux・macOS では、実行権限を付けて `PATH` の通ったディレクトリに配置します。
次は `~/.local/bin` を使う例です。

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 <ファイル名> "$HOME/.local/bin/rev-callgraph"
"$HOME/.local/bin/rev-callgraph" --version
```

Windows では、照合済みの ZIP を PowerShell で展開して実行します。

```powershell
Expand-Archive rev-callgraph-windows-amd64.zip -DestinationPath rev-callgraph
.\rev-callgraph\rev-callgraph.exe --version
```

### Go でインストールする

Go 1.27.4 以降を使います。

```bash
go install github.com/meian/rev-callgraph@latest
rev-callgraph --version
```

## 使い方

```bash
rev-callgraph github.com/meian/rev-callgraph/testdata/foo.Target --dir ./testdata
rev-callgraph github.com/meian/rev-callgraph/testdata/foo.Target --dir ./testdata --format json --json-style edges
```

target は関数なら `<package>.<FuncName>`、メソッドなら `<package>.<TypeName>#<MethodName>` です。既定は runtime symbol・実行環境の GOOS/GOARCH・tree 出力です。テスト中の呼び出しを含める場合は `--symbol-set test`、対象環境を指定する場合は `--goos` と `--goarch` を使います。

すべてのフラグと例は [使い方](docs/usage.md)、解析対象・系列判定・出力の詳細は [仕様](docs/spec.md) を参照してください。
探索と追加解析を繰り返す流れは [図で追う解析の全体像](docs/how-it-works.md) を参照してください。
