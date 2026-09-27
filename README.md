# rev-callgraph

Go の関数やメソッドを起点に、ワークスペース内の呼び出し元を逆向きにたどる CLI です。複数 module を含むディレクトリを解析でき、同じ major 系列なら依存 version の minor・patch が異なっていても候補に含めます。

## インストール

Go 1.26 以降で実行します。

```bash
go install github.com/meian/rev-callgraph@latest
```

## 使い方

```bash
rev-callgraph github.com/meian/rev-callgraph/testdata/foo.Target --dir ./testdata
rev-callgraph github.com/meian/rev-callgraph/testdata/foo.Target --dir ./testdata --format json --json-style edges
```

target は関数なら `<package>.<FuncName>`、メソッドなら `<package>.<TypeName>#<MethodName>` です。既定は runtime symbol・実行環境の GOOS/GOARCH・tree 出力です。テスト中の呼び出しを含める場合は `--symbol-set test`、対象環境を指定する場合は `--goos` と `--goarch` を使います。

すべてのフラグと例は [使い方](docs/usage.md)、解析対象・系列判定・出力の詳細は [仕様](docs/spec.md) を参照してください。
