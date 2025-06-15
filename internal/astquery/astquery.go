package astquery

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/meian/rev-callgraph/internal/contextutil"
	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/meian/rev-callgraph/internal/progress"
	"github.com/meian/rev-callgraph/internal/symbol"
)

// determinePkgPath はファイルパスからモジュールのルートと相対パスを用いてパッケージの import パスを決定します
func determinePkgPath(filePath string, mm gomod.ModuleMap) string {
	best := ""
	root := ""
	// 対象モジュールを走査
	// ルートディレクトリが深いものから返されるので、最初に見つかったものを選択
	for _, m := range mm.Iter {
		rel, err := filepath.Rel(m.Root, filepath.Dir(filePath))
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		best = m.Path
		root = m.Root
		break
	}
	if best == "" {
		return filepath.Base(filepath.Dir(filePath))
	}
	relPath, _ := filepath.Rel(root, filepath.Dir(filePath))
	if relPath == "." {
		return best
	}
	return best + "/" + filepath.ToSlash(relPath)
}

// ExtractCallers は target を呼び出す関数/メソッドのリストを返します。
// target の書式は "pkg.Func" または "pkg.Type#Method" です。
func ExtractCallers(ctx context.Context, target string, files []string, modules gomod.ModuleMap) ([]symbol.Function, error) {
	progress.Msgf(ctx, "extract callers for %s", target)
	var callers []symbol.Function
	for _, file := range files {
		if contextutil.IsCanceledOrTimedOut(ctx) {
			return nil, ctx.Err()
		}
		// ファイルパース
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("ASTパース失敗 %s: %w", file, err)
		}

		// インポートマップ: エイリアスまたはパッケージ名 -> モジュールパス
		importMap := make(map[string]string)
		for _, imp := range node.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			alias := ""
			if imp.Name != nil && imp.Name.Name != "_" {
				alias = imp.Name.Name
			} else {
				parts := strings.Split(impPath, "/")
				alias = parts[len(parts)-1]
			}
			importMap[alias] = impPath
		}

		// ファイルのモジュールパスを決定
		pkgPath := determinePkgPath(file, modules)

		// 関数/メソッド宣言ごとにボディ内を探索
		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			// 呼び出し元の名前を構築
			var callerName string
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				// メソッド: レシーバ型を抽出
				recvType := fn.Recv.List[0].Type
				var typeName string
				switch expr := recvType.(type) {
				case *ast.StarExpr:
					if ident, ok := expr.X.(*ast.Ident); ok {
						typeName = ident.Name
					}
				case *ast.Ident:
					typeName = expr.Name
				}
				callerName = fmt.Sprintf("%s.%s#%s", pkgPath, typeName, fn.Name.Name)
			} else {
				// 関数
				callerName = fmt.Sprintf("%s.%s", pkgPath, fn.Name.Name)
			}

			// AST走査で CallExpr を検出
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// 呼び出し式の関数部分を文字列化して比較
				var name string
				switch fun := ce.Fun.(type) {
				case *ast.SelectorExpr:
					if pkgIdent, ok := fun.X.(*ast.Ident); ok {
						if impPath, exists := importMap[pkgIdent.Name]; exists {
							name = fmt.Sprintf("%s.%s", impPath, fun.Sel.Name)
						} else if fun.Sel.Name == baseName(target) {
							// ローカル変数を通じたメソッド呼び出しをターゲットにマッチ
							name = strings.ReplaceAll(target, "#", ".")
						} else {
							name = fmt.Sprintf("%s.%s", pkgPath, fun.Sel.Name)
						}
					} else {
						if fun.Sel.Name == baseName(target) {
							// ネストされたセレクタによるターゲットメソッド呼び出し
							name = strings.ReplaceAll(target, "#", ".")
						} else {
							name = fmt.Sprintf("%s.%s", pkgPath, fun.Sel.Name)
						}
					}
					if name == strings.ReplaceAll(target, "#", ".") {
						if fnSym, err := symbol.ParseFunction(callerName); err == nil {
							callers = append(callers, fnSym)
						}
					}
				case *ast.Ident:
					if fun.Name == baseName(target) {
						if fnSym, err := symbol.ParseFunction(callerName); err == nil {
							callers = append(callers, fnSym)
						}
					}
				}
				return true
			})
		}
	}
	for _, c := range callers {
		progress.Msgf(ctx, "  caller: %s", c)
	}
	return callers, nil
}

// baseName は target から関数/メソッド名部分を抽出します
func baseName(target string) string {
	if idx := strings.LastIndexAny(target, ".#"); idx >= 0 {
		return target[idx+1:]
	}
	return target
}
