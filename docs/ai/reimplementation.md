# rev-callgraph フルスクラッチ再実装

## 目的

`rev-callgraph` をフルスクラッチで再実装する。

既存実装の内部設計・コード構成・実装方法は引き継がない。

Git履歴、過去のPR、過去のIssue、既存コード上の設計判断についても、原則として新実装の設計根拠にはしない。

既存リポジトリから継承するものは以下とする。

- 現在提供しているCLI機能
- CLIの入力仕様
- CLIの出力仕様
- 現在利用者から観測できる振る舞い
- rev-callgraph が本来解決しようとしている解析要件

つまり、

> 外部仕様と解析要件は継承するが、内部実装は完全に作り直す。

ことを前提とする。

既存コードは「どう実装するか」の参考にはしない。
必要な場合のみ「現在どう振る舞っているか」を確認する目的で参照する。

---

# 既存仕様の確認

実装開始前に、現在の外部仕様を確認する。

主な確認対象:

- README
- CLI help
- E2Eテスト
- golden test
- testdata
- 現行CLIの実行結果

既存コードは、外部仕様を上記だけでは確認できない場合に限って参照する。

既存実装のpackage構成、関数分割、アルゴリズムをそのまま移植しない。

---

# CLI互換性

原則として現在のCLI入出力を変更しない。

現在のtarget指定、既存flag、tree / JSON / DOT等の出力形式を維持する。

ただし、この仕様で明示的に追加・変更を許可しているものは例外とする。

現在明示的に追加を許可するもの:

- 解析対象symbol setの指定
- target GOOSの指定
- target GOARCHの指定
- compatibility / resolution情報を表現するために必要な出力拡張

それ以外の外部仕様変更が必要と判断した場合は、独断で変更しない。

変更前に以下を整理する。

1. 現在の仕様
2. 変更案
3. 変更理由
4. 互換性への影響

---

# 最重要の解析要件

rev-callgraph の目的は、

> 現在の依存関係をそのまま `go build` できるコードだけを解析すること

ではない。

複数のGo moduleが存在するワークスペースにおいて、依存moduleの使用versionが完全一致していなくても、通常の依存更新対象として同一系列とみなせるsource間について逆call graphを追跡できることを目的とする。

例えば、

```text
service-a
  require example.com/common v1.2.x

service-b
  require example.com/common v1.5.x

workspace上の common
  v1系相当
```

のような状態でも、version完全一致を要求しない。

---

# module系列の定義

ここでいう系列は、API互換性、source互換性、binary互換性を意味しない。

基準は、

> 通常の依存更新作業において `go get` による更新対象として同一系列とみなす範囲

とする。

依存version更新後にinterface変更等でbuild errorが発生する可能性は、解析対象から除外する理由にしない。

通常の更新作業では、

```text
go get
↓
compile error等を確認
↓
変更されたinterfaceに利用側を追従
```

という作業が発生し得るためである。

系列判定で判断するのは、

> 現在そのままbuildできるか

ではなく、

> 同一の依存系列として更新対象になり得るか

である。

## 同一系列

以下は同一系列として扱う。

```text
v0.1.x → v0.2.x
v0.2.x → v0.9.x

v1.1.x → v1.2.x
v1.2.x → v1.10.x

v2.1.x → v2.8.x
```

minor / patchの完全一致は要求しない。

minor間でAPIが破壊されていても、同一major内なら解析対象とする。

特に `v0` 系についても、

```text
v0.x → v0.y
```

を同一系列として扱う。

## 別系列

majorが変わる場合は別系列とする。

```text
v0 → v1
v1 → v2
v2 → v3
```

`v0 → v1` も別系列である。

GoのSemantic Import Versioningによって、

```text
example.com/module/v2
```

のようにmodule pathが変わるケースも別系列として扱う。

---

# version不整合時の解析

例えば、

```text
consumer
  require common v1.2

workspace
  common v1.7相当
```

であっても、双方が同じ `v1` 系なら解析対象とする。

ただし、

- function削除
- method削除
- 引数変更
- 戻り値変更
- 型変更

などによってcall siteが現在のworkspace側sourceに対して成立しない可能性がある。

それは解析対象から除外する理由ではなく、解析結果として表現する。

一部の型整合性が崩れていても、解析可能な範囲まで全体を継続する。

---

# 解析方式

具体的な解析方式は固定しない。

以下は要件ではない。

- 最初に文字列検索する
- ASTだけで解析する
- SSAを使用しない
- DFSで都度sourceを再検索する
- `rg` を利用する

要件を満たせるのであれば、

- `go/ast`
- `go/types`
- `go/packages`
- SSA
- 軽量source scan
- 独自index
- `ripgrep`
- 複数方式の組み合わせ

などを選択してよい。

ただし、

> workspace全体を単一の型整合したprogramとしてロードできないことを理由に、解析可能なmoduleまで失敗させない。

完全な型解析が成立しない場合も、package単位の解析、AST情報、独自resolution等を用いて解析可能な範囲を残す。

---

# 独自解析モデル

ASTやSSA等のライブラリ固有型を、解析後のアプリケーション全体へ持ち込まない。

概念的な処理境界は、

```text
Go source
↓
source analysis
↓
rev-callgraph独自モデル
↓
symbol / type / call resolution
↓
compatibility check
↓
reverse call graph
↓
formatter
```

とする。

`ast.Node`、`ast.Expr`、`ast.FuncDecl`、`ssa.Function` 等を、後続処理の共通データモデルとして使用しない。

独自モデルとして少なくとも以下を検討する。

- module
- module series / version
- package
- source file
- build context
- symbol set
- function
- method
- receiver
- import
- symbol
- symbol reference
- call
- resolution
- compatibility
- graph node
- graph edge

不必要に巨大なモデルにはしない。

後続処理に必要な情報を基準に定義する。

---

# ResolutionとCompatibilityを分離する

「call先をどこまで解決できたか」と、

「現在のcallee定義に対してcall siteが成立するか」

は別概念として扱う。

## Resolution Status

少なくとも以下を区別する。

```text
resolved
external
unknown
```

解析途中の内部状態として必要なら、

```text
unresolved / not-loaded
```

も別に持つ。

### resolved

rev-callgraphの通常解析対象内に定義が存在し、さらに解析を継続できる。

### external

シンボルやsignature等は把握できているが、rev-callgraphの解析範囲外なのでそこで解析を終了する。

つまり、

> 何者かは分かっているが、それ以上は追わない

状態。

### unknown

必要な追加探索を行っても、十分な情報を取得できなかった状態。

つまり、

> まだ読んでいない

ことと、

> 探したが分からない

ことを区別する。

基本フロー:

```text
unresolved
↓
追加探索
├─ resolved
├─ external
└─ 解決不能 → unknown
```

---

# Compatibility Status

call edgeごとに、可能な範囲で以下を判定する。

```text
compatible
incompatible
unknown
```

Resolution Statusとは別軸である。

例えば、

```text
resolution = external
compatibility = compatible
```

は有効な状態とする。

---

# call site互換性判定

caller側の現在のcall siteが、workspace上のcallee定義に対して成立するかを可能な範囲で判定する。

例えば、

```go
result := common.Find(id)
```

に対して現在のcalleeが、

```go
func Find(ctx context.Context, id string) (*Item, error)
```

なら、

```text
caller → common.Find
```

という関係そのものは結果に残す。

その上で、

```text
compatibility = incompatible
```

と理由を保持する。

少なくとも以下を検出対象として検討する。

- function / method消失
- 引数数変更
- 引数型変更
- variadic変更
- 戻り値数変更
- 戻り値型変更
- receiver / method set変更
- その他Goの型情報から明確にcall不成立と判断できる変更

非互換理由は単なるbooleanではなく、構造化して保持する。

---

# 非互換時のreverse traversal

初期実装では、`incompatible` なedgeを探索境界とする。

例えば、

```text
A
↓
B
↓ incompatible
C
↓
Target
```

なら、Targetから逆方向に、

```text
Target
└─ C
   └─ B [incompatible]
```

までは結果に含める。

B自身は結果に残すが、それより上位のAは探索しない。

将来的には、

> incompatibleでもさらに上位まで解析を継続する

オプションを追加できる構造にする。

初期CLIでそのオプションを公開する必要はない。

ただし、traversal policyとして解析ロジックから分離する。

---

# unknown時の探索

原則として、

```text
compatible   → 継続
incompatible → 停止
unknown      → 継続
```

とする。

`unknown` は「明確に壊れている」ことを意味しないためである。

---

# cgo

cgoを一律 `unknown` にしない。

`import "C"` のpreambleやinclude先には、

- C function declaration
- C function definition
- typedef
- struct
- macro
- header include

等が存在し得る。

例えば、

```go
/*
int foo(int);
*/
import "C"

func F() {
    C.foo(1)
}
```

で `foo` の宣言を認識できる場合、

```text
resolution = external
```

として扱える。

signatureが取得でき、

```go
C.foo(1)
```

が成立すると判断できれば、

```text
resolution    = external
compatibility = compatible
```

とできる。

C内部のcall graphまでrev-callgraphが追跡しない場合、`C.foo` を終端として扱う。

宣言等を追加探索しても情報を取得できない場合のみ `unknown` とする。

必要ならexternal理由として、

```text
cgo-preamble
cgo-header
cgo-library
other-external
```

等を表現できる構造にする。

分類名や粒度は実装時に調整してよい。

---

# Symbol Set

テストを「test fileを読むかどうか」だけで扱わず、解析対象となるsymbol setとして扱う。

初期実装では、

```text
runtime
test
```

を提供する。

CLIは、

```text
--symbol-set runtime
--symbol-set test
```

を基本とする。

## runtime

デフォルト。

通常のbuild時に利用可能なsymbol集合。

`*_test.go` にのみ存在するsymbolは含めない。

## test

test実行時に利用可能になるsymbol集合。

概念的には、

```text
runtime symbols
+
*_test.go
+
external test package
```

となる。

`TestXxx`、`BenchmarkXxx`、`FuzzXxx`、`ExampleXxx` 等はtest symbol setに含まれる。

benchmark専用symbol setは設けない。

---

# Target Build Context

Symbol SetとBuild Contextは別軸にする。

概念的には、

```text
AnalysisContext
├─ SymbolSet
│  ├─ runtime
│  └─ test
└─ BuildContext
   ├─ GOOS
   ├─ GOARCH
   ├─ cgo
   └─ build tags
```

とする。

初期CLIでは、

```text
--goos
--goarch
```

を追加する。

## target未指定

GOOS / GOARCH等を指定しない場合のみ、実行環境のbuild contextをデフォルトとして利用する。

## target指定

例えばhostが、

```text
darwin/arm64
```

でも、

```text
--goos linux --goarch amd64
```

を指定した場合は、

```text
linux/amd64
```

のみを対象とする。

host側の `darwin` / `arm64` を解析条件へ混ぜない。

以下を正しく考慮できる構造にする。

- GOOS
- GOARCH
- filename suffix constraints
- `//go:build`
- cgo
- custom build tags

初期CLIですべてを公開する必要はない。

---

# 大規模packageとLazy Analysis

packageに数百の `.go` fileが存在する場合でも、最初から全fileを完全解析することを前提にしない。

目的は、

> call graphの一部を調べるためだけにpackage全体をAST解析・type check・SSA構築するコストを常に支払わないこと

である。

基本フロー:

```text
target解析
↓
必要source解析
↓
symbol / call解決
↓
情報不足
↓
必要symbol/typeの定義を探索
↓
該当sourceのみ追加解析
↓
解決
```

とする。

---

# Source DiscoveryとSource Analysis

以下を分離する。

```text
SourceLocator
    ↓
definition candidate

SourceAnalyzer
    ↓
source model

SymbolResolver
    ↓
resolved / external / unknown
```

## SourceLocator

責務:

> symbol / type等の定義が存在する可能性のあるsource fileを特定する。

## SourceAnalyzer

責務:

> 特定されたsource fileを詳細解析し、rev-callgraph独自モデルへ変換する。

SourceLocatorが詳細解析まで担当したり、SourceAnalyzerがpackage全体探索を担当したりしない。

---

# SourceLocatorの差し替え

Source Discoveryの具体的な実装方式は固定しない。

将来的に、

```text
InProcessLocator
RipgrepLocator
IndexedLocator
```

等へ差し替え可能な責務境界を持たせる。

候補には、

- Goによる軽量scan
- token scan
- 限定的AST利用
- 文字列検索
- `ripgrep`
- 独自symbol index

等がある。

ただし、初期実装から複数Locatorを作る必要はない。

重要なのは、

> SourceLocator利用側が、候補sourceをどう発見したかを知らなくてよい

ことである。

差し替え可能性のためだけに、

- 不要なinterface
- abstract factory
- plugin framework
- 未使用実装
- 過度なDI

を作らない。

---

# cache / index

同一実行中に、一度解析・解決したものを不要に再処理しない。

少なくとも以下をcache対象として検討する。

```text
source file
→ parsed source model

symbol
→ resolution

type
→ resolution result

call site
→ compatibility result

module/package
→ discovery information
```

SourceLocatorについても、

```text
SymbolRef
→ candidate source
```

を再利用できる構造にする。

将来 `rg` を使用する場合でも、symbol lookupごとに毎回外部processを起動することを前提にしない。

cache keyには必要に応じて、

- module系列
- package
- symbol set
- target build context

を含める。

一方、contextに依存しないsource parse結果等は不必要に複製しない。

---

# CLIは薄くする

CLIに解析ロジックを持たせない。

基本的には、

```text
引数・flag取得
↓
validation
↓
AnalysisContext生成
↓
application処理呼び出し
↓
formatter
↓
stdout / stderr
```

のみを担当する。

CLIを起動しなくても、主要機能を通常のGo関数としてテストできる構造にする。

---

# 処理構造

概念的には、

```text
CLI
 ↓
workspace discovery
 ↓
module resolver
 ↓
analysis context resolver
 ↓
source locator
 ↓
source analyzer
 ↓
source model
 ↓
symbol / type resolver
 ↓
call resolver
 ↓
compatibility checker
 ↓
reverse call graph builder
 ↓
formatter
```

と責務を分離する。

これはpackage構成を固定するものではない。

packageを細かく分割すること自体を目的にしない。

---

# ドキュメント

人が読むものとAIが読むものを分離する。

## README

`README.md` は人が最初に読む入口。

以下程度に留める。

- rev-callgraphが何をするツールか
- installation
- 最小限のusage
- 代表的なexample
- 主要option
- 詳細文書へのリンク

内部architectureを大量に記載しない。

目的:

> 初めて使う人が短時間で使い始められること。

## 人向けUsage

例えば、

```text
docs/usage.md
```

目的:

> CLI利用者が目的に応じて正しいコマンドを実行できること。

記載例:

- target指定
- flag
- symbol set
- GOOS / GOARCH
- tree / JSON / DOT
- max depth
- runtime / test解析
- 使用例

## 人向けSpecification

例えば、

```text
docs/spec.md
```

目的:

> rev-callgraphの外部から観測できる仕様を正確に確認できること。

記載例:

- target syntax
- module系列判定
- Symbol Set
- Build Context
- resolution status
- compatibility status
- cgo / externalの扱い
- reverse traversal
- output schema

内部実装方式は原則書かない。

## AI向けドキュメント

```text
docs/ai/
```

に配置する。

目的:

> AIが実装、変更、レビューする際に、architectureと不変条件を理解すること。

記載対象:

- architecture
- package責務
- dependency direction
- source model
- SourceLocator / SourceAnalyzer
- lazy resolution
- cache
- Resolution / Compatibility
- traversal policy
- AnalysisContext
- Symbol Set
- Build Context
- テスト方針
- 人向け文書との対応

---

# ドキュメント同期

実装または外部仕様を変更した場合、

```text
実装
テスト
人向けドキュメント
AI向けドキュメント
```

を同一変更の一部として扱う。

AI向け文書には、

> 実装または外部仕様を変更した場合、影響する人向け文書とAI向け文書も同じ変更内で更新すること。

と明記する。

人向け文書にも、実装・仕様・関連ドキュメントを同期管理する旨を簡潔に記載する。

READMEを保守ルールで肥大化させない。

---

# テスト方針

主要処理をCLIなしで単体テスト可能にする。

少なくとも以下を独立してテスト可能にする。

- target parse
- workspace discovery
- module解析
- module系列判定
- AnalysisContext
- Symbol Set
- Build Context
- SourceLocator
- SourceAnalyzer
- source → 独自model
- symbol resolution
- type resolution
- external resolution
- call resolution
- compatibility
- reverse graph
- traversal policy
- cycle
- max depth
- formatter

---

# version関連テスト

少なくとも以下を含める。

```text
same version
minor差異
patch差異
v0内のminor差異
v0 → v1
v1 → v2
複数consumerが異なるminorを要求
```

同一major内はAPI互換性に関係なく解析対象。

majorが異なれば別系列。

---

# compatibilityテスト

少なくとも以下を含める。

- 引数追加
- 引数削除
- 引数型変更
- variadic変更
- 戻り値追加
- 戻り値削除
- 戻り値型変更
- receiver変更
- method消失
- function消失
- 型情報不足
- API変更後もcall siteが成立するケース

非互換caller自身は結果に含め、その先のreverse traversalを停止する。

---

# Symbol Setテスト

## runtime

`*_test.go` 由来のcallerを含めない。

## test

通常codeとtest codeの双方を含める。

## external test package

`package foo_test` も適切に扱う。

## cache

runtime / test間でresolution結果を不適切に共有しない。

---

# Build Contextテスト

例えばhostが、

```text
darwin/arm64
```

でもtargetとして、

```text
linux/amd64
```

を指定した場合、linux/amd64のみを解析する。

host contextを混入させない。

以下の組み合わせを確認する。

```text
linux/amd64 + runtime
linux/amd64 + test
```

build constraintsも適切に反映する。

---

# cgo / externalテスト

preamble declaration:

```go
/*
int foo(int);
*/
import "C"

func F() {
    C.foo(1)
}
```

について、宣言が取得できる場合は、

```text
resolution = external
```

とする。

signatureが判定できる場合は、

```text
compatibility = compatible
```

も判定できる。

preamble definitionやheader由来の宣言も同様に扱う。

追加探索しても解決不能なら、

```text
resolution = unknown
```

とし、externalと混同しない。

---

# Lazy Resolutionテスト

例えば、

```go
func A(v Foo) {
    B(v)
}
```

を含むsourceだけを先に解析し、`Foo` が別fileに存在する場合、

```text
Fooが必要
↓
SourceLocator
↓
定義source発見
↓
SourceAnalyzer
↓
型解決
```

できること。

以下も確認する。

- 未ロードとunknownを区別する
- 不要なsourceを完全解析しない
- 定義なしならunknownになる
- 追加解析後にcompatibilityを再判定できる

---

# 大規模packageテスト

例えば、

```text
001.go
002.go
...
300.go
```

が存在し、実際の解析に必要なのが、

```text
012.go
087.go
201.go
```

のみとなるfixtureを用意する。

以下を確認する。

- 不要sourceを完全解析しない
- 必要sourceのみ追加解析する
- 同じsourceを不要に再解析しない

可能であれば、

```text
解析source数
SourceLocator探索回数
SourceAnalyzer実行回数
symbol resolution回数
cache hit/miss
```

等をテストまたはbenchmarkから観測可能にする。

---

# 出力

既存の正常系出力は可能な限り維持する。

resolution / compatibilityを利用者が確認できるようにする。

JSONでは例えば、

```json
{
  "caller": "...",
  "callee": "...",
  "resolution": {
    "status": "external",
    "kind": "cgo-header"
  },
  "compatibility": {
    "status": "compatible",
    "issues": []
  }
}
```

のように、resolutionとcompatibilityを独立して表現できる構造を検討する。

tree / DOTについても、非互換・external・unknown等が必要に応じて確認できるようにする。

既存schemaの変更範囲は最小限にする。

---

# 設計上の優先順位

1. 同一major系列をversion完全一致なしで横断できる
2. call relationの解析精度
3. API非互換箇所を検出できる
4. resolution failureと意図的な解析境界を区別できる
5. 一部が型不整合でも解析可能な範囲を失わない
6. 必要source中心のlazy analysis
7. Symbol Set / Build Contextを正しく分離する
8. 処理の流れと責務が明確
9. テスト容易性
10. パフォーマンス

lazy analysis可能なarchitectureは最初から用意する。

一方、

- `rg`
- 独自index
- 並列scan
- その他の高速化

は実測結果を見て導入する。

---

# コード品質

以下を重視する。

- 入力から結果まで処理を追いやすい
- AST/SSA等の知識を解析全体へ漏らさない
- ResolutionとCompatibilityを混同しない
- externalとunknownを混同しない
- unresolvedとunknownを混同しない
- 一つの関数へ複数責務を詰め込まない
- 副作用を局所化する
- package間依存を単純にする
- テストのためだけの不自然なinterfaceを増やさない
- 将来差し替える可能性が高い責務だけ適切に分離する
- premature optimizationを避ける
- コメントで補わないと理解できない構造より、コード構造自体を分かりやすくする

---

# 実装の進め方

いきなり既存コードを書き換え始めない。

最初に、

1. 現行外部仕様を確認
2. この仕様と現行仕様の差分を整理
3. 新しい独自データモデルを設計
4. component/package責務を設計
5. dependency directionを設計
6. test strategyを整理
7. 実装
8. テスト
9. documentation同期
10. 全体review

の順で進める。

ただし、設計だけ提示して作業を終了しない。

同一タスク内で実装まで完了する。

リポジトリに定義されたサブエージェントを、作業内容に応じて利用してよい。

全体architecture、共通モデル、責務境界、重要な統合判断は親エージェントが保持する。

---

# 完了条件

以下を満たした状態を完了とする。

- 既存実装をベースにせずフルスクラッチで再実装されている
- 明示的に変更した部分以外のCLI互換性が維持されている
- 同一major内でminor / patchが異なっても追跡できる
- `v0 → v1` を含むmajor変更は別系列
- API互換性をmodule系列判定条件にしない
- 一部が型不整合でも解析可能な範囲を失わない
- AST / SSA等の固有型が後続主要処理へ漏れていない
- source解析結果が独自モデルになっている
- `resolved / external / unknown` を区別できる
- unresolvedとunknownを区別できる
- ResolutionとCompatibilityが別々に表現される
- `compatible / incompatible / unknown` を表現できる
- 非互換理由を構造化して保持できる
- 非互換edgeで初期状態のreverse traversalを停止する
- traversal policyが解析処理から分離されている
- cgoを一律unknown扱いしない
- external symbolを終端として表現できる
- runtime / test Symbol Setを切り替えられる
- runtimeへtest-only symbolが混入しない
- target GOOS / GOARCHを指定できる
- target指定時にhostのGOOS / GOARCHを混入しない
- Symbol SetとBuild Contextが別概念
- package全体の完全解析を常に要求しない
- 必要sourceを追加解析できる
- Source DiscoveryとSource Analysisが分離されている
- SourceLocatorの探索実装を利用側が意識しない
- 将来in-process / rg / index等へ差し替え可能
- 初期実装で不要な過剰抽象化を行わない
- 同一sourceを不要に再解析しない
- contextを考慮してcacheを安全に利用する
- CLIに解析ロジックが存在しない
- READMEが人向けの短い入口になっている
- 人向けUsageとSpecificationが目的別に分離されている
- AI向けドキュメントが人向けと分離されている
- 実装・テスト・人向け文書・AI向け文書が同期している
- 主要処理をCLIなしで単体テストできる
- CLI E2Eテストが存在する
- 大規模packageで不要sourceの完全解析を抑制できることをテストできる
- `go test ./...` が成功する
- `go vet ./...` が成功する
- 入力 → AnalysisContext → discovery → analysis → resolution → compatibility → graph → output の流れをコードから容易に追える
