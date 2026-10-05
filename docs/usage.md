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

import の名前を省略した場合は、import path に対応するファイルの `package` 宣言名を使います。
ディレクトリ末尾と宣言名が違っても、ターゲットには import path を指定します。
外部テスト内の関数・型は同じ外部テスト package から参照でき、通常の import の候補には入りません。

例えば `foo/` の外部テストと通常の `foo_test/` に同名関数 `SameName` がある場合、`--symbol-set test` での `example.com/m/foo_test.SameName` は曖昧です。
CLI/API は `ambiguous target` と候補の所在地を返します。
通常 package だけを調べる場合は `--symbol-set runtime` を使えます。
所在地を追加してターゲットを指定する構文は提供していません。
別の一意なターゲットからの結果に両者が現れた場合は、`example.com/m/foo_test.SameName [/absolute/path/foo; external-test]` と `example.com/m/foo_test.SameName [/absolute/path/foo_test; package]` のように表示します。
この付記は同じ結果内で公開 ID が重なる場合だけで、tree・JSON・DOT に共通です。

```bash
rev-callgraph example.com/app.Target --dir ./workspace --symbol-set test --goos linux --goarch amd64
rev-callgraph example.com/app.Target --dir ./workspace --format dot > graph.dot
rev-callgraph example.com/app.Target --dir ./workspace --format json --json-style edges > graph.json
rev-callgraph example.com/app.Target --dir ./workspace --max-depth 2 --progress
```

tree は起点から呼び出し元を字下げして表示します。JSON `nested` は `name` と `callers` の木、JSON `edges` は `root`、`nodes`、`edges` の集合です。DOT は `caller -> callee` の有向辺を出力します。通常の解決済みで互換な辺は付加情報を省き、それ以外は resolution と compatibility を表示します。同じ caller に互換・非互換の呼び出しが混在する場合は、判定ごとに辺や木の枝を分けて表示します。詳細は [仕様](spec.md) を参照してください。

機能を変更するときは、実装・テスト・この文書・[仕様](spec.md)・[AI 向け設計](ai/architecture.md) を同じ変更内で更新します。
