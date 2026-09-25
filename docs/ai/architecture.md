# 解析アーキテクチャ

実装または外部仕様を変更した場合、影響する人向け文書と AI 向け文書も同じ変更内で更新すること。

## 境界と流れ

`cmd` は入力・flag の検証、`analysis.Options` の構築、`output.Write` の呼び出しだけを担当する。主要な処理は `analysis.Analyze(ctx, target, options)` で CLI なしに実行できる。

```text
CLI -> Discover -> NewLocator -> AnalyzeSource (必要な file)
    -> source model -> resolve -> compatibility -> reverse traversal
    -> output.Write
```

`internal/analysis/model.go` の `Workspace`、`SourceModel`、`Function`、`Call`、`Resolution`、`Compatibility`、`Node`、`Edge` が処理間のモデルである。`go/ast` の型は `AnalyzeSource` の内部で使い、後続へ渡さない。`TypeRef` は名前・定数値・未解決の field 参照を持ち、別 file の型定義が必要になった時点で追加解決する。`internal/output` は結果を tree、JSON、DOT に変換する。旧解析・出力 package は削除し、CLI の実行経路をこの構成に一本化した。

## Discovery と lazy analysis

`Discover` は `go.mod` を収集し、nested module 境界を確定してから、symbol set と build context に合う Go source を列挙する。module path の major suffix を優先し、suffix がなければ workspace の require major から source module の系列を推定する。一意でない系列は空値のままとする。`replace` は旧 version・新 path・新 version を保持し、現在の require に適用できるものだけを解決と系列推定に利用する。解析時には major の完全一致を要求するが minor・patch は要求しない。`v0` と `v1` は分離する。

`NewLocator` は対象 file の token を一度走査して、定義名と識別子名から候補 file の index を作る。`Definitions(packagePath, name)` と `Callers(packagePath, name)` は候補を返すだけで、呼び出しと確定しない。`Callers` は見落としを防ぐため package を越えて広めに候補を返す。`AnalyzeSource` は要求された file だけを AST 解析し、独自モデルへ変換する。関数値とメソッド式は別 file の定義でも symbol 名をモデルに残す。`callersFor` は起点の symbol 名と無関係な call を解決前に除外し、同じ caller が他の関数を多数呼んでいても不要な定義 source を詳細解析しない。定義、source model、外部宣言、symbol ごとの caller と互換性判定の結果は `engine` の実行単位で cache する。`Statistics` の `DiscoveredSources`、`AnalyzedSources`、`LocatorLookups`、`ResolutionLookups`、`CacheHits` でこの動作を検証できる。

## Resolution、Compatibility、traversal

`resolve` は追加 source の読込後に `resolved`、`external`、`unknown` を決める。未ロードの状態を失敗とみなさない。ワークスペース内の定義、alias、field と embedded method、interface の宣言、標準ライブラリ宣言、cgo 宣言を扱う。標準ライブラリにも同一の target build context を渡す。外部探索は package/name/receiver を照合し、receiver を保持した独自 Function を cache する。source 変換では block に加えて制御文と case/communication 節の scope を出入りし、init 宣言による shadow が文外へ漏れないようにする。cgo は preamble と単純な local header 宣言を読む。宣言の取れない外部 symbol は `unknown` とし、存在を仮定した `external` にしない。

`compatibility` は resolution とは独立して、既知の signature、引数、結果、method expression などを比較する。明確な不一致は `Issue{Kind, Message}` とともに `incompatible`、型情報が足りない場合は `unknown` にする。`TraversalPolicy` は非互換辺を越えるかどうかを切り替える。CLI の初期設定では非互換 caller を残してその先を停止し、`unknown` は継続する。call site ごとの判定後、同じ caller/callee・resolution・compatibility・issues の辺のみ表示をまとめる。異なる判定をまとめて非互換を優先してはならない。cycle と max depth は graph 構築時に処理する。

## テストと同期

target parse、module 系列、symbol set、build context、locator、source model、resolution、compatibility、traversal、formatter を CLI から独立してテストする。E2E は flag と標準出力・標準エラーの契約を確認する。大規模 package では候補 file だけを詳細解析した数を確認する。

実装を変更したら関連テストを実行し、利用者に見える挙動は [使い方](../usage.md) と [仕様](../spec.md)、最初の入口は [README](../../README.md) と同期する。

## 実装時に確定した事項

- 外部仕様は README、help、E2E、現行 CLI の出力から確認した。既存内部アルゴリズムは移植していない。
- 旧 CLI では help に掲載された DOT が未実装だった。再実装では要件に従い DOT を提供する。
- 正常辺の JSON schema を保つため resolved / compatible は省略可能とし、それ以外だけ metadata を追加する。
- version のない local module が v0 と v1 の両方から参照される場合、source の major は確定できない。Git 履歴から推測せず、その系列を要する module 間接続を作らない。
- 旧実装に依存していた単体テストは削除し、同じ外部挙動の検証を新しい analysis のテストと CLI E2E に移した。

## レビュー指摘の回帰確認

- `source_regression_test.go`: 別 file の関数値・メソッド式、配列長の保持と情報不足。
- `array_identity_test.go`: 配列要素の alias・定義型の区別、pointer・ネスト配列。
- `interface_regression_test.go`: 埋め込み method、pointer method set、遮蔽、曖昧さ。
- `discovery_test.go`: version 限定 replace の適用条件・優先順位・置換先系列。
- `engine_review_test.go`: 混在する call site の経路保持、無関係な300関数の詳細解析抑制。
- `e2e_test.go`: 互換・非互換の両方を JSON に残し、正常経路の上位 caller を保持。

- `scope_regression_test.go`: 制御文と各節の局所宣言、else・nested scope、文外での import 解決。
- `external_method_test.go`: receiver 別の外部宣言、メソッド式、target context、external 終端と未解決時の継続。
