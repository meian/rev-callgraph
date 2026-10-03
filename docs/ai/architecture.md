# 解析アーキテクチャ

実装または外部仕様を変更した場合、影響する人向け文書と AI 向け文書も同じ変更内で更新すること。

## 境界と流れ

`cmd` は入力・flag の検証、`analysis.Options` の構築、`output.Write` の呼び出しだけを担当する。主要な処理は `analysis.Analyze(ctx, target, options)` で CLI なしに実行できる。

```text
CLI -> Discover (module 発見 -> source 列挙と package 構築)
    -> NewLocator (file の token index)
    -> target 定義探索 -> callersFor (候補 file の AnalyzeSource)
    -> resolve -> compatibility -> reverse traversal
    -> output.Write
```

`internal/analysis/model.go` の `Workspace`、`Package`、`SourceModel`、`Function`、`Call`、`Resolution`、`Compatibility`、`Node`、`Edge` が処理間のモデルである。`go/ast` の型は `AnalyzeSource` の内部で使い、後続へ渡さない。`TypeRef` は名前・定数値・未解決の field 参照を持ち、別 file の型定義が必要になった時点で追加解決する。`internal/output` は結果を tree、JSON、DOT に変換する。旧解析・出力 package は削除し、CLI の実行経路をこの構成に一本化した。

## Discovery と lazy analysis

### 状態と所有者

1 回の `AnalyzeWithPolicy` は、指定された build context と symbol set に対して新しい `Workspace`、`Locator`、`engine` を作る。異なる解析条件の間で cache は共有しない。`discoverModules` は `go.mod` を発見し module の系列と nested module の所有境界を確定する。`discoverSources` は対象条件に合う file を列挙し、package 宣言と import header を読み、`Workspace.Packages` を作る。package の file 所属は path・所有 module・directory・external test の区別で管理する。通常 file と internal test file は同じ package、external test file は別 package に属する。`Package.ID` と各 `Source.PackageID` は、この区別を含む一回の解析内だけの identity である。`bar` の external test と通常の `bar_test` directory はどちらも従来の symbol path `module/bar_test` を持つが、異なる `Package.ID` に属するため `Sources` が混在しない。同名の `main` package でも import path は directory ごとに異なる。

locator と関数・型の ID は引き続き従来の package path を使う。このため、上記の external test と通常 package が同名の symbol を持つ場合などの ID 衝突は既存の制約として残る。解析用 identity を locator・symbol・cache に一貫して伝える対応は #80 の後続範囲とする。

`Workspace.Sources` は所在確認済みの file、`NewLocator` の成功後はその全 file の token index が構築済みとなる。`Locator.Definitions` は package path と symbol 名から、`Locator.Callers` は symbol 名から候補 file を返す。候補は確定した定義・caller ではない。package から探索を始め、必要な候補 file だけを `engine.load` が詳細解析する。`engine.models` に path がなければ詳細解析は未実施、あれば `SourceModel` が完成している。`functions` と `types` は完成した file model から作る。新しい source I/O 削減や package の遅延発見はここでは行わない。

`definitionCache` は package path と名前、`callerCache` は完全な symbol ID を key とする。両者は探索と必要な file の読み込みが成功した後だけ記録する。`models` にも成功した解析結果だけを記録する。未ロードは不存在を意味しない。候補を読み終えた後に初めて「定義なし」と判断でき、`resolve` は workspace 内 package の symbol 不在を `unknown/missing-symbol`、外部宣言を確認できない場合を `unknown/definition-unavailable` とする。file の読み込み・解析が失敗した場合はエラーを返し、完了 cache を記録しないので同じ解析条件で再試行できる。module/source 発見や index 構築の失敗時は engine を作らず解析全体を失敗させる。

caller 計算結果は同じ engine の固定された候補集合に対して再利用する。後続の別条件や追加発見された package に持ち越さない。逆方向 traversal の cycle は現在経路の ancestors、max depth は現在経路の深さで判定し、別経路で同じ symbol に到達しても分岐を残す。表示用の同等辺だけをまとめる。

定義が見つからない場合の package 判定は、同じ path のうち caller の所有 module を優先し、`unknown/missing-symbol` を返す。caller の module に一致する package がない場合は、同じ path の候補から互換系列の module を探し、1 つでもあれば `unknown/missing-symbol` を返す。候補があり、どれも非互換系列だった場合だけ `external/different-module-series` とする。

`Discover` は `go.mod` を収集し、nested module 境界を確定してから、symbol set と build context に合う Go source を列挙する。module path の major suffix を優先し、suffix がなければ workspace の require major から source module の系列を推定する。一意でない系列は空値のままとする。`replace` は旧 version・新 path・新 version を保持し、現在の require に適用できるものだけを解決と系列推定に利用する。解析時には major の完全一致を要求するが minor・patch は要求しない。`v0` と `v1` は分離する。

`NewLocator` は対象 file の token を一度走査して、定義名と識別子名から候補 file の index を作る。`Definitions(packagePath, name)` と `Callers(packagePath, name)` は候補を返すだけで、呼び出しと確定しない。`Callers` は見落としを防ぐため package を越えて広めに候補を返す。`AnalyzeSource` は要求された file だけを AST 解析し、独自モデルへ変換する。関数値とメソッド式は別 file の定義でも symbol 名をモデルに残す。`callersFor` は起点の symbol 名と無関係な call を解決前に除外し、同じ caller が他の関数を多数呼んでいても不要な定義 source を詳細解析しない。定義、source model、外部宣言、symbol ごとの caller と互換性判定の結果は `engine` の実行単位で cache する。`Statistics` の `DiscoveredSources`、`AnalyzedSources`、`LocatorLookups`、`ResolutionLookups`、`CacheHits` でこの動作を検証できる。

## Resolution、Compatibility、traversal

import 名は明示 alias、workspace の package 宣言名、標準ライブラリの package 宣言名の順で取得する。標準ライブラリの名前取得には解析対象の build context を渡し、取得できなければ import path の末尾へフォールバックする。通常 source と外部宣言で同じ名前取得を使用し、`math/rand/v2` のように path 末尾と宣言名が異なる package を扱う。

`resolve` は追加 source の読込後に `resolved`、`external`、`unknown` を決める。未ロードの状態を失敗とみなさない。ワークスペース内の定義、alias、field と embedded method、interface の宣言、標準ライブラリ宣言、cgo 宣言を扱う。標準ライブラリにも同一の target build context を渡す。外部探索は package/name/receiver を照合し、receiver を保持した独自 Function を cache する。外部宣言の parse 失敗も nil として cache し、初回・再利用時とも `unknown` / `definition-unavailable` を返す。source 変換では block に加えて制御文と case/communication 節の scope を出入りし、init 宣言による shadow が文外へ漏れないようにする。cgo は preamble と単純な local header 宣言を読む。宣言の取れない外部 symbol は `unknown` とし、存在を仮定した `external` にしない。

`compatibility` は resolution とは独立して、既知の signature、引数、結果、method expression などを比較する。メソッド式の第一引数には `call.Receiver` を使い、その型の method set を検証する。非互換でも解決できた関係は残すため、通常のメソッド呼び出しと共通の宣言探索では値・ポインタの差だけを理由に排除しない。明確な不一致は `Issue{Kind, Message}` とともに `incompatible`、型情報が足りない場合は `unknown` にする。`TraversalPolicy` は非互換辺を越えるかどうかを切り替える。CLI の初期設定では非互換 caller を残してその先を停止し、`unknown` は継続する。call site ごとの判定後、同じ caller/callee・resolution・compatibility・issues の辺のみ表示をまとめる。異なる判定をまとめて非互換を優先してはならない。cycle と max depth は graph 構築時に処理する。

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

- `method_expression_regression_test.go`: 昇格メソッド式、値・ポインタのmethod set、正常経路と非互換境界。
- `external_cache_test.go`: 外部宣言のparse失敗を同一実行内で再解析しないこととunknown理由の一貫性。
- `package_name_regression_test.go`: 標準ライブラリの宣言package名、明示alias、workspace名の優先順位と対象build context。
- `channel_regression_test.go`: channelの方向、定義型とalias、入れ子channelを含む要素型の同一性、互換経路の継続と非互換境界。
- `external_alias_regression_test.go`: 標準ライブラリ型のalias経由のメソッド、連鎖・ポインタ・メソッド式、独立した定義型の除外。
