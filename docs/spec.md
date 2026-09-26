# 外部仕様

rev-callgraph は指定した関数・メソッドへ到達する呼び出し元を逆向きにたどります。target は関数が `<package>.<FuncName>`、メソッドが `<package>.<TypeName>#<MethodName>` です。対象 symbol の定義が現在の source に無くても、呼び出し元を示せる場合はその target を起点にします。

## 解析対象

`--dir` 以下の `go.mod` を再帰的に発見し、各 source を最も内側の module に所属させます。`vendor` と `.git` は走査しません。nested module は親 module から独立して扱います。

module 系列は、module path の `/v2` 以降の suffix または `gopkg.in` の `.vN` を優先します。それがなければ、ワークスペース内の `require` version の major から一意に推定します。local `replace` で指された module もその対象です。発見済みの置換先は import の解決にも反映します。version 限定の `replace` は選択中の `require` version に一致するときだけ適用し、全 version 向けの指定より優先します。置換先 version が指定されている場合は、その major を系列判定に用います。同じ major なら minor・patch 差は許容します。`v0` と `v1` は異なる系列です。参照が無い場合や `v0` と `v1` の要求が混在する場合など、系列を一意に決められない module は不明として扱い、系列が必要な module 間接続を推測しません。系列は API 互換性の保証ではありません。

symbol set は `runtime`（既定）と `test` です。`runtime` は `*_test.go` を除外します。`test` はそれらと `package foo_test` の external test package を含めます。対象 GOOS/GOARCH を指定すると、その環境に合う filename suffix と `//go:build` 条件の source を選びます。未指定の軸には実行環境の値を使います。cgo と custom build tags は内部の build context で扱いますが、初期 CLI に専用フラグはありません。対象 GOOS/GOARCH が実行環境と異なる場合、cgo は既定で無効です。

## 解決と互換性

各 call edge は別々の `resolution` と `compatibility` を持ちます。

| 項目 | 値 | 意味 |
| --- | --- | --- |
| resolution | `resolved` | ワークスペース内で定義が見つかり、探索可能 |
| resolution | `external` | 宣言を把握できるが、探索対象外の境界 |
| resolution | `unknown` | 必要な探索後も定義情報が不足 |
| compatibility | `compatible` | 現在の定義に対して確認した範囲で成立 |
| compatibility | `incompatible` | 現在の定義との明確な不一致を検出 |
| compatibility | `unknown` | 型や宣言などの情報不足で判定不能 |

解析中の「未ロード」は `unknown` と区別します。欠落した symbol、引数数や既知の型、variadic、結果数や既知の結果型などを検査し、非互換の理由を `issues` に保持します。型情報が不足する場合は無理に非互換と判定せず `unknown` とします。定数の表現可能範囲、判明している配列長、値型・ポインタ型および埋め込みによる method set も検査します。別ファイルの定義を参照する関数値呼び出しとメソッド式も、定義を追加探索して解決します。制御文や各節で宣言した変数はその有効範囲に限って名前を隠し、文の外側の import・symbol 解決へ持ち越しません。配列長などの型情報を確定できない場合は `unknown` とします。完全な Go type checker による build 成否判定ではありません。reflection や動的な関数値の全候補列挙、複雑な generic constraint の証明は行わず、情報が不足する判定は `unknown` にします。

cgo は `import "C"` に対応し、preamble や source と同じディレクトリにある直接 include された header の単純な C 宣言から名前と引数型を読める範囲で `external` と互換性を判定します。複雑な macro、C の全構文や header の依存連鎖を網羅するものではなく、情報を取得できなければ `unknown` です。標準ライブラリも指定した build context で関数・receiver 付きメソッドの宣言を探し、参照できた場合は `external` として終端にします。外部宣言を解析できない場合は `unknown` / `definition-unavailable` として探索を継続し、同一実行内で失敗した宣言を繰り返し解析しません。同名の関数と別の型のメソッドを区別し、メソッド式では式に書かれた receiver 型を第一引数の型として扱い、その型の method set に対象メソッドが含まれるかを別途検査します。値 receiver のメソッドは `T.M` と `(*T).M` の双方で利用できますが、ポインタ receiver のみのメソッドを `T.M` として利用する式は非互換です。

逆探索は `compatible` と `unknown` の辺では継続します。`incompatible` の caller 自身は結果へ残し、その caller より上位では停止します。`external` は終端です。同一 caller から同一 callee への呼び出しでも、解決・互換性・非互換理由が異なる場合は別の辺・木の枝として残します。そのため一部の call site が非互換でも、別の互換な call site を通る上位探索は継続します。判定が同じ呼び出しは表示をまとめます。cycle は表示して再帰を止めます。`--max-depth` が正の値なら起点を深さ 0 として探索を制限し、`0` 以下なら深さ制限はありません。

## 出力

`--format tree`（既定）は起点と呼び出し元の木を 2 スペースずつ字下げします。main と cycle にはそれぞれ `[main]`、`(cycled)` を付けます。通常の `resolved` / `compatible` 辺以外には `[resolution=..., compatibility=...]` と issue の kind を付けます。

`--format json` の既定 `nested` は次の形です。

```json
{"name":"example.com/app.Target","callers":[{"name":"example.com/app.Caller"}]}
```

`--json-style edges` は `root`、辞書順の `nodes`、`caller`・`callee` の `edges` を持ちます。通常の `resolved` / `compatible` 辺では `resolution` と `compatibility` を省略し、欠けている値はこの組み合わせを意味します。その他の辺では、両者を独立した object として出力し、`compatibility.issues` に構造化した理由を載せます。`nested` ではこの metadata を該当 caller node に載せます。`main` と `cycled` は true の場合にのみ載せます。

`--format dot` は `digraph rev_callgraph` を出力し、辺は `caller -> callee` です。通常の辺以外は `resolution / compatibility` の label を付けます。

出力形式を変える場合は、実装・テスト・[使い方](usage.md)・この仕様・[AI 向け設計](ai/architecture.md) を同じ変更内で同期します。
