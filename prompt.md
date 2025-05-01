# GitHub Copilot エージェント用プロンプト

> **目的**: Go 1.24 以降で実装した CLI ツール **`rev-callgraph`** を開発し、指定ワークスペース内で関数/メソッドを逆方向にたどるコールグラフを構築・可視化する。メジャーバージョンが一致するモジュール間を横断し、マイナーバージョン以下の差異は無視して解析する。

---

## 1. 背景・前提

- **言語**: Go (>= 1.24.0)
- **モジュール名**: `github.com/meian/rev-callgraph`
- **実行形態**: クロスプラットフォーム CLI (`go install` 可能)
- **ワークスペース**: 任意ディレクトリ (フラグ `--dir`、省略時はカレントディレクトリ)。複数モジュールが階層的／横断的に配置されている可能性を考慮する。
- **解析対象**: 必須位置引数 **`<target>`** に指定された **関数 or メソッド** からの **逆** コールグラフ (呼び出し元チェーン)。
- **バージョン解決ルール**:
  - **0.x 系** → すべて同一バージョンとして扱う。
  - **1.x 以降** → SemVer の *major* 部分のみ一致を要求し、`minor`/`patch`/`prerelease` は無視。

### 1‑1. 解析対象の指定ルール

| 種別                      | 記法                                              | 例                             |
| ------------------------- | ------------------------------------------------- | ------------------------------ |
| **パッケージ (内部利用)** | `モジュール + ルートからの階層をスラッシュで連結` | `example.com/hoge/foo`         |
| **関数**                  | `<package>"."<FuncName>`                          | `example.com/hoge/foo.Bar`     |
| **メソッド**              | `<package>"."<TypeName>"#"<MethodName>`           | `example.com/hoge/foo.Bar#Qux` |

> `#` 区切りはメソッド、`.` 区切りは関数を示す。CLI ではこれらの完全パスをそのまま `<target>` として渡す。

---

## 2. 要件

### 2‑1. CLI インタフェース (Cobra ベース)

> **cobra‑cli** によるコマンドツリーを生成済みと仮定し、`cmd/` 配下にサブコマンドを実装する。

```bash
$ rev-callgraph <target> \
    --dir ./path/to/workspace \
    --format json   # json | tree | dot (tree が既定)
```

> **TODO**: `--format`, `--json-style`, `--max-depth`, `--exact-ssa` (将来予定) など追加フラグを検討。

### 2‑2. 機能概要

1. **モジュールスキャン (軽量アプローチ)**

   - `filepath.WalkDir` でワークスペース直下の **`go.mod`** を列挙。
   - `golang.org/x/mod/modfile` で各 `go.mod` を解析し、`module` 名と `require` 行を取得。
   - 得られた情報で **ワークスペース内モジュールマップ** を構築 (依存 → 被依存)。
   - **`go.work`**** 等の外部ファイルは生成しない**。ワークスペース内のコードのみを対象に解析。

2. **逆コールチェーン抽出 (AST ベース)**

   **フェーズ A : 文字列検索で絞り込み**

   - `<target>` をワークスペース内 `.go` ファイルへ **単語境界ベースで高速検索**。
     - メソッド指定の場合 `#` を `.` に変換して同時検索 (`Bar#Qux` → `Bar.Qux`)。
   - マッチしたファイルパスを収集。

   **フェーズ B : AST 解析で呼び出し確認**

   - フェーズ A の各ファイルについて `go/parser.ParseFile` と `go/ast` で AST を生成。
   - `ast.Inspect` で以下を走査:
     - `*ast.CallExpr` : `selector or ident` が **関数指定** (`pkg.Func`) と一致するか。
     - `*ast.SelectorExpr` + `*ast.CallExpr` : `recv.Type` とメソッド名が **メソッド指定** (`pkg.Type#Method`) と一致するか。
   - 一致した場合、その **呼び出し側関数/メソッド** (`*ast.FuncDecl` or `*ast.FuncLit`) を取得し、呼び出しチェーンに追加。
   - 取得した呼び出し側を次ターゲットとしてフェーズ A へ戻る (深さ優先 DFS)。
   - 型情報に依存しないため、インタフェース経由の動的呼び出しなどは解析外と割り切る。

3. **結果フォーマット**

   > **内部管理は常に ****`edges`**** 形式、出力は人間向けに ****`nested`**** を既定とする**。

   - **edges (内部構造)**
     ```json
     {
       "nodes": ["pkg.Func", "caller1", "caller2"],
       "edges": [
         {"caller": "caller1", "callee": "pkg.Func"},
         {"caller": "caller2", "callee": "caller1"}
       ],
       "root": "pkg.Func"
     }
     ```
     - CLI では `--json-style edges` でダンプ可能 (デバッグ／機械処理用)。
   - **nested (既定出力)**
     ```json
     {
       "name": "pkg.Func",
       "callers": [
         {
           "name": "caller1",
           "callers": [
             {
               "name": "caller2",
               "callers": [],
               "cycled": true  // サイクル検出で打ち切り
             }
           ]
         }
       ]
     }
     ```
     - 深さ優先でツリー化。呼び出し履歴に **すでに登場したノード** に再度到達した場合は `{ "cycled": true }` を立ててそれ以上の出力を省略。
     - `--no-cycle-mark` で `cycled` フラグを省くオプションを将来追加しても良い。
   - **tree**: ASCII ツリー。
   - **dot**: Graphviz DOT。

4. **テスト/CI** **テスト/CI**

   - `testdata/` にマルチモジュールサンプルを配置し、E2E テストを構築。
   - GitHub Actions で `go vet`, `staticcheck`, `go test ./...` を実行。

---

## 3. 実装ステップ (Copilot へのタスク分割指示)

| Step | タスク                                                                                           | 主要パッケージ                                          |
| ---- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------- |
| 1    | **プロジェクト雛形生成**: `go mod init`, `cobra-cli init` *(cobra‑cli は環境にインストール済み)* | `github.com/spf13/cobra`                                |
| 2    | **go.mod スキャナ実装**: `WalkDir` + `x/mod/modfile` でモジュールマップを構築                    | `golang.org/x/mod/modfile`                              |
| 3    | **文字列検索ユーティリティ**: Go 実装の簡易 grep (`strings.Contains`, `bufio.Scanner`)           |                                                         |
| 4    | **AST 解析器**: 絞込ファイルのみ `go/parser`, `go/ast` で呼び出し抽出                            | `go/parser`, `go/ast`                                   |
| 5    | **コールグラフ逆走アルゴリズム**: DFS で呼び出し元チェーンを列挙                                 |                                                         |
| 6    | **フォーマッタ**: `json`, `tree`, `dot` 出力                                                     | `encoding/json`, `github.com/awalterschulze/gographviz` |
| 7    | **統合テスト & パフォーマンス計測** *(TODO)*                                                     | `testing`, `pprof`                                      |

---

## 4. 受け入れ基準

- 指定ターゲット(関数またはメソッド)に到達する呼び出しチェーンを深さ優先ですべて列挙できること。
- `go.mod` ベースのモジュール解決が機能し、0.x & major 一致ルールが反映されていること。
- 文字列検索→AST 解析の 2 段階によって余分なパースが抑制されていること。
- `--format` ごとに正しい出力が得られること。
- `go vet`/`staticcheck` にパスすること。

---

## 5. 出力例 (JSON / nested)

```json
{
  "name": "example.com/hoge/foo.Bar#Qux",
  "callers": [
    {
      "name": "example.com/hoge/foo.main",
      "callers": []
    }
  ]
}
```

---

## 6. コーディングガイドライン

- [Effective Go](https://go.dev/doc/effective_go) & [Code Review Comments](https://go.dev/wiki/CodeReviewComments) 準拠。
- `goimports` でフォーマット、`staticcheck` で静的解析。
- パフォーマンスクリティカルな箇所は `pprof` で計測しつつ最適化。

---

以上の要件に従い、GitHub Copilot へ段階的に指示して実装を進めてください。



---

## README ドラフト

以下は `rev-callgraph` の README としてそのまま使用できる Markdown 例です。

```markdown
# rev-callgraph

Go 1.24+ で動作する “逆向き” コールグラフ解析 CLI （ワークスペース内の複数モジュールを横断し、major 一致だけを条件に解析）

## インストール

```bash
go install github.com/meian/rev-callgraph@latest
```

## ディレクトリ構成（cobra‑cli 生成後 + 主要パッケージ）

```text
rev-callgraph/
├── cmd/                 # cobra サブコマンド群
│   ├── root.go          # フラグ定義・エントリポイント
│   └── callgraph.go     # core: 逆コールグラフ抽出ロジック呼び出し
├── internal/
│   ├── modscan/         # go.mod スキャン & モジュールマップ
│   ├── grep/            # 文字列検索ユーティリティ
│   ├── astquery/        # AST 解析 & 呼び出し検出
│   ├── graph/           # edges 構造体 & DFS
│   └── format/          # nested/tree/dot エンコーダ
├── testdata/            # E2E サンプル
├── go.mod
├── main.go
└── README.md
```

## 使い方

```bash
rev-callgraph <target> [flags]
```

| フラグ         | デフォルト | 説明                                       |
| -------------- | ---------- | ------------------------------------------ |
| `--dir`        | `.`        | 解析するワークスペースのルートディレクトリ |
| `--format`     | `json`     | 出力形式 `json` / `tree` / `dot`           |
| `--json-style` | `nested`   | `json` でのスタイル `nested` / `edges`     |
| `--max-depth`  | `0`        | 逆探索の最大深さ (`0` は制限なし)          |
| `--version`    |            | バージョン表示                             |
| `-h`, `--help` |            | ヘルプ表示                                 |

### `<target>` の書式

| 種別     | 記法                                | 例                             |
| -------- | ----------------------------------- | ------------------------------ |
| 関数     | `<package>.<FuncName>`              | `example.com/hoge/foo.Bar`     |
| メソッド | `<package>.<TypeName>#<MethodName>` | `example.com/hoge/foo.Bar#Qux` |

> **パッケージ部分**は「モジュールパス + ルートからの階層」を `/` で連結した完全 import path。

## 出力例

### JSON (`nested` / デフォルト)

```json
{
  "name": "example.com/hoge/foo.Bar#Qux",
  "callers": [
    {
      "name": "example.com/hoge/foo.Process",
      "callers": [
        {
          "name": "main.main",
          "callers": [],
          "cycled": true
        }
      ]
    }
  ]
}
```

### JSON (`edges` / 内部表現・`--json-style edges`)

```json
{
  "nodes": [
    "example.com/hoge/foo.Bar#Qux",
    "example.com/hoge/foo.Process",
    "main.main"
  ],
  "edges": [
    { "caller": "example.com/hoge/foo.Process", "callee": "example.com/hoge/foo.Bar#Qux" },
    { "caller": "main.main", "callee": "example.com/hoge/foo.Process" }
  ],
  "root": "example.com/hoge/foo.Bar#Qux"
}
```

