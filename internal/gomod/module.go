package gomod

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/meian/rev-callgraph/internal/symbol"
)

// Module はモジュールパスとその依存先を表します
type Module struct {
	// Path はモジュールパス
	Path string
	// Requires は依存モジュールパス一覧
	Requires []string
	// Root は go.mod のディレクトリ（モジュールのルート）
	Root string
}

// ContainsPackage は pkg がモジュールに含まれるかを判定します
// pkg で示すパッケージがモジュールのルートまたは子パッケージである場合、true を返します
func (m Module) ContainsPackage(pkg string) bool {
	return pkg == m.Path ||
		strings.HasPrefix(pkg, m.Path+"/")
}

// PackageDir は pkg が配置されるディレクトリを返します
// pkg がモジュールに属さない場合はエラーを返します
func (m Module) PackageDir(pkg string) (string, error) {
	if !m.ContainsPackage(pkg) {
		return "", errors.New("not a child package")
	}
	if pkg == m.Path {
		return m.Root, nil
	}
	return filepath.Join(m.Root, strings.TrimPrefix(pkg, m.Path+"/")), nil
}

// HasDefinition は指定された関数/メソッド定義がこのモジュール内に存在するか判定します
// f.PkgPathがモジュール内に存在しなければfalse, 存在すればパッケージ内の.goファイルを走査して定義を探す
func (m Module) HasDefinition(f symbol.Function) (bool, error) {
	if !m.ContainsPackage(f.PkgPath) {
		return false, nil
	}
	pkgDir, err := m.PackageDir(f.PkgPath)
	if err != nil {
		return false, err
	}
	files, err := os.ReadDir(pkgDir)
	if err != nil {
		return false, err
	}
	var pat *regexp.Regexp
	if f.IsMethod() {
		// メソッド定義: func (recv Type) Method( または func (recv *Type) Method(
		pat = regexp.MustCompile(`func\s*\([^)]+\*?` + regexp.QuoteMeta(f.TypeName) + `\)\s*` + regexp.QuoteMeta(f.Name) + `\s*\(`)
	} else {
		// 関数定義: func FuncName(
		pat = regexp.MustCompile(`func\s+` + regexp.QuoteMeta(f.Name) + `\s*\(`)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pkgDir, file.Name()))
		if err != nil {
			continue // 読めないファイルはスキップ
		}
		if pat.Match(data) {
			return true, nil
		}
	}
	return false, nil
}

// ModuleMap はモジュールのマップを表します
type ModuleMap struct {
	mmap  map[string]Module
	paths []string
}

// NewModuleMap は新しい ModuleMap を作成します
func NewModuleMap(m map[string]Module) *ModuleMap {
	paths := make([]string, 0, len(m))
	for k := range m {
		paths = append(paths, k)
	}
	slices.SortFunc(paths, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	return &ModuleMap{
		mmap:  m,
		paths: paths,
	}
}

// FindByPath は pkg で指定したパッケージを含む Module を検索します
func (mm ModuleMap) FindByPackage(pkg string) (*Module, bool) {
	for _, p := range mm.paths {
		if mm.mmap[p].ContainsPackage(pkg) {
			mod := mm.mmap[p]
			return &mod, true
		}
	}
	return nil, false
}

// FindByFunctionは与えられた関数/メソッド定義が存在するModuleを検索します
// まずパッケージを含むモジュールを特定し、その中で定義を探索します
// 見つからない場合は(nil, nil)、エラー時は(nil, err)を返します
func (mm ModuleMap) FindByFunction(f symbol.Function) (*Module, error) {
	mod, ok := mm.FindByPackage(f.PkgPath)
	if !ok {
		return nil, nil // パッケージを含むモジュールが存在しない
	}
	exists, err := mod.HasDefinition(f)
	if err != nil {
		return nil, err // ファイルアクセス等のエラー
	}
	if !exists {
		return nil, nil // 定義が見つからない
	}
	return mod, nil
}

// ReferencedBy は target で指定したモジュールを require しているモジュール一覧を返します
func (mm ModuleMap) ReferencedBy(m Module) []Module {
	var result []Module
	for _, p := range mm.paths {
		mod := mm.mmap[p]
		for _, req := range mod.Requires {
			if req == m.Path {
				result = append(result, mod)
				break
			}
		}
	}
	return result
}

// Iter はモジュールのキーと値をイテレートするイテレータ関数です
// 返されるモジュールはパスの長さの降順(同じ長さの場合は辞書順)でソートされます
func (mm ModuleMap) Iter(yield func(string, Module) bool) {
	for _, key := range mm.paths {
		if !yield(key, mm.mmap[key]) {
			return
		}
	}
}

// Len はモジュールの数を返します
func (mm ModuleMap) Len() int {
	return len(mm.paths)
}
