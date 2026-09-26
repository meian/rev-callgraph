# 使い方

```bash
rev-callgraph <target> [flags]
```

`<target>` は、関数なら `<package>.<FuncName>`、メソッドなら `<package>.<TypeName>#<MethodName>` です。`<package>` には module path を含む Go package の import path を指定します。

```bash
rev-callgraph github.com/meian/rev-callgraph/testdata/foo.Target --dir ./testdata
rev-callgraph github.com/meian/rev-callgraph/testdata/foo.SomeStruct#Method --dir ./testdata
```

| フラグ | 既定値 | 内容 |
| --- | --- | --- |
| `--dir` | `.` | 再帰的に `go.mod` と source を探すディレクトリ |
| `--format` | `tree` | `tree`、`json`、`dot` |
| `--json-style` | `nested` | JSON の `nested` または `edges`。`--format json` のときに使用 |
| `--max-depth` | `0` | 逆探索の最大深さ。`0` 以下は無制限 |
| `--progress` | `false` | 解析開始と解析済み source 数を標準エラーへ表示 |
| `--symbol-set` | `runtime` | `runtime` または `test` |
| `--goos` | 実行環境 | 対象 GOOS |
| `--goarch` | 実行環境 | 対象 GOARCH |

`--symbol-set test` は通常の source に加えて `*_test.go` と external test package を含みます。`--goos` と `--goarch` は対象 build context の選択で、symbol set とは独立しています。

```bash
rev-callgraph example.com/app.Target --dir ./workspace --symbol-set test --goos linux --goarch amd64
rev-callgraph example.com/app.Target --dir ./workspace --format dot > graph.dot
rev-callgraph example.com/app.Target --dir ./workspace --format json --json-style edges > graph.json
rev-callgraph example.com/app.Target --dir ./workspace --max-depth 2 --progress
```

tree は起点から呼び出し元を字下げして表示します。JSON `nested` は `name` と `callers` の木、JSON `edges` は `root`、`nodes`、`edges` の集合です。DOT は `caller -> callee` の有向辺を出力します。通常の解決済みで互換な辺は付加情報を省き、それ以外は resolution と compatibility を表示します。同じ caller に互換・非互換の呼び出しが混在する場合は、判定ごとに辺や木の枝を分けて表示します。詳細は [仕様](spec.md) を参照してください。

機能を変更するときは、実装・テスト・この文書・[仕様](spec.md)・[AI 向け設計](ai/architecture.md) を同じ変更内で更新します。
