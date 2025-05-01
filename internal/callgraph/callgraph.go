package callgraph

import (
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/meian/go-rev-callgraph/internal/astquery"
	"github.com/meian/go-rev-callgraph/internal/gomod"
	"github.com/meian/go-rev-callgraph/internal/grep"
	"github.com/meian/go-rev-callgraph/internal/symbol"
)

// getPkgNameFromPath はパッケージパスからGoのpackage宣言名を取得します
func getPkgNameFromPath(pkgPath string, mods gomod.ModuleMap) string {
	mod, ok := mods.FindByPackage(pkgPath)
	if !ok {
		return ""
	}
	dir, err := mod.PackageDir(pkgPath)
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.PackageClauseOnly)
		if err == nil && file.Name != nil {
			return file.Name.Name
		}
	}
	return ""
}

// CallersTree はツリー構造で呼び出し元を再帰的に構築する
// maxDepth=0の場合は無制限
func CallersTree(mod gomod.Module, target string, mods gomod.ModuleMap, depth int, seen map[string]struct{}, maxDepth int) (*symbol.CallNode, error) {
	if seen == nil {
		seen = make(map[string]struct{})
	}

	// mainパッケージか判定
	isMain := false
	if idx := strings.LastIndexAny(target, ".#"); idx > 0 {
		pkgPath := target[:idx]
		isMain = getPkgNameFromPath(pkgPath, mods) == "main"
	}

	// サイクル検出
	if _, exists := seen[target]; exists {
		return &symbol.CallNode{Name: target, Cycled: true, Main: isMain}, nil
	}
	// 最大深さに到達したら探索終了（0は無制限）
	if maxDepth > 0 && depth >= maxDepth {
		return &symbol.CallNode{Name: target, Main: isMain}, nil
	}
	seen[target] = struct{}{}

	var callers []*symbol.CallNode

	// 同一モジュール内を探索
	files, err := grep.SearchFiles(mod.Root, target)
	if err != nil {
		return nil, fmt.Errorf("grep.SearchFiles失敗: %w", err)
	}
	callerList, err := astquery.ExtractCallers(target, files, mods)
	if err != nil {
		return nil, fmt.Errorf("AST解析失敗: %w", err)
	}
	for _, c := range callerList {
		modOfCaller, err := mods.FindByFunction(c)
		if err == nil && modOfCaller != nil {
			child, err := CallersTree(*modOfCaller, c.String(), mods, depth+1, CloneSeen(seen), maxDepth)
			if err == nil && child != nil {
				callers = append(callers, child)
			}
		}
	}

	// 参照元モジュールを探索
	for _, refMod := range mods.ReferencedBy(mod) {
		files, err := grep.SearchFiles(refMod.Root, target)
		if err != nil {
			continue // 参照元1つ失敗しても他は続行
		}
		callerList, err := astquery.ExtractCallers(target, files, mods)
		if err != nil {
			continue
		}
		for _, c := range callerList {
			modOfCaller, err := mods.FindByFunction(c)
			if err == nil && modOfCaller != nil {
				child, err := CallersTree(*modOfCaller, c.String(), mods, depth+1, CloneSeen(seen), maxDepth)
				if err == nil && child != nil {
					callers = append(callers, child)
				}
			}
		}
	}
	return &symbol.CallNode{Name: target, Callers: callers, Main: isMain}, nil
}

// CloneSeen はサイクル検出用マップをコピーする
func CloneSeen(seen map[string]struct{}) map[string]struct{} {
	return maps.Clone(seen)
}
