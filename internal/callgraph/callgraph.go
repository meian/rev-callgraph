package callgraph

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/meian/rev-callgraph/internal/astquery"
	"github.com/meian/rev-callgraph/internal/contextutil"
	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/meian/rev-callgraph/internal/grep"
	"github.com/meian/rev-callgraph/internal/progress"
	"github.com/meian/rev-callgraph/internal/symbol"
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
func CallersTree(ctx context.Context, mod gomod.Module, target string, mods gomod.ModuleMap, depth int, seen map[string]struct{}, maxDepth int) (*symbol.CallNode, error) {
	if contextutil.IsCanceledOrTimedOut(ctx) {
		return nil, ctx.Err()
	}
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
		progress.Msgf(ctx, "cycle detected for %s in %s", target, mod.Path)
		return &symbol.CallNode{Name: target, Cycled: true, Main: isMain}, nil
	}
	// 最大深さに到達したら探索終了（0は無制限）
	if maxDepth > 0 && depth >= maxDepth {
		progress.Msgf(ctx, "max depth reached for %s in %s", target, mod.Path)
		return &symbol.CallNode{Name: target, Main: isMain}, nil
	}

	progress.Msgf(ctx, "search callers for %s in %s", target, mod.Path)
	seen[target] = struct{}{}

	var callers []*symbol.CallNode

	// 同一モジュール内を探索
	files, err := grep.SearchFiles(ctx, mod.Root, target)
	if err != nil {
		return nil, fmt.Errorf("grep.SearchFiles失敗: %w", err)
	}
	callerList, err := astquery.ExtractCallers(ctx, target, files, mods)
	if err != nil {
		return nil, fmt.Errorf("AST解析失敗: %w", err)
	}
	for _, c := range callerList {
		if contextutil.IsCanceledOrTimedOut(ctx) {
			return nil, ctx.Err()
		}
		modOfCaller, err := mods.FindByFunction(ctx, c)
		if err == nil && modOfCaller != nil {
			child, err := CallersTree(ctx, *modOfCaller, c.String(), mods, depth+1, cloneSeen(seen), maxDepth)
			if err != nil {
				continue
			}
			callers = append(callers, child)
		}
	}

	// 参照元モジュールを探索
	for _, refMod := range mods.ReferencedBy(mod) {
		if contextutil.IsCanceledOrTimedOut(ctx) {
			return nil, ctx.Err()
		}
		progress.Msgf(ctx, "search for referenced module: %s", refMod.Path)
		files, err := grep.SearchFiles(ctx, refMod.Root, target)
		if err != nil {
			continue // 参照元1つ失敗しても他は続行
		}
		callerList, err := astquery.ExtractCallers(ctx, target, files, mods)
		if err != nil {
			continue
		}
		for _, c := range callerList {
			if contextutil.IsCanceledOrTimedOut(ctx) {
				return nil, ctx.Err()
			}
			modOfCaller, err := mods.FindByFunction(ctx, c)
			if err == nil && modOfCaller != nil {
				child, err := CallersTree(ctx, *modOfCaller, c.String(), mods, depth+1, cloneSeen(seen), maxDepth)
				if err != nil {
					continue
				}
				callers = append(callers, child)
			}
		}
	}

	return &symbol.CallNode{Name: target, Callers: callers, Main: isMain}, nil
}

// cloneSeen はサイクル検出用マップをコピーする
func cloneSeen(seen map[string]struct{}) map[string]struct{} {
	return maps.Clone(seen)
}
