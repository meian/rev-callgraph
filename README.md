# rev-callgraph

Go言語で指定した関数またはメソッドについて呼び出し元を探索する逆向きのコールグラフを解析するCLIツール

## インストール

Go 1.24以降が利用できる環境で以下のコマンドを実行する

```bash
go install github.com/meian/rev-callgraph@latest
```

## 使い方

```bash
rev-callgraph <target> [flags]
```

| フラグ         | デフォルト | 説明                                       |
| -------------- | ---------- | ------------------------------------------ |
| `--dir`        | `.`        | 解析するワークスペースのルートディレクトリ |
| `--format`     | `tree`     | 出力形式: `json` / `tree` / `dot`         |
| `--json-style` | `nested`   | JSONスタイル: `nested` (ツリー) / `edges`  |
| `--max-depth`  | `0`        | 逆探索の最大深さ (`0` は制限なし)          |

### `<target>` の書式

| 種別     | 記法                                | 例                                                                          |
| -------- | ----------------------------------- | --------------------------------------------------------------------------- |
| 関数     | `<package>.<FuncName>`              | `github.com/meian/rev-callgraph/testdata/foo.Target`                     |
| メソッド | `<package>.<TypeName>#<MethodName>` | `github.com/meian/rev-callgraph/internal/astquery.ExtractCallers#Invoke` |

## 出力例

### tree
```
github.com/meian/rev-callgraph/testdata/foo.Target
  github.com/meian/rev-callgraph/testdata/foo.CallTarget
  github.com/meian/rev-callgraph/testdata/bar.Caller
    github.com/meian/rev-callgraph/testdata/qux.Caller
      github.com/meian/rev-callgraph/testdata/app.main
```

### JSON (nested / デフォルト)
```json
{
  "name": "github.com/meian/rev-callgraph/testdata/foo.Target",
  "callers": [
    {
      "name": "github.com/meian/rev-callgraph/testdata/bar.Caller",
      "callers": []
    }
  ]
}
```

### JSON (edges)
```json
{
  "root": "github.com/meian/rev-callgraph/testdata/foo.Target",
  "nodes": [
    "github.com/meian/rev-callgraph/testdata/bar.Caller",
    "github.com/meian/rev-callgraph/testdata/foo.Target"
  ],
  "edges": [
    {"caller":"github.com/meian/rev-callgraph/testdata/bar.Caller","callee":"github.com/meian/rev-callgraph/testdata/foo.Target"}
  ]
}
```
