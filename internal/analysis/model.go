// Package analysis はソースコードに基づく逆呼び出し解析を実装する。
// 特定のライブラリに依存する構文木は、ソース解析の境界を越えない。
package analysis

import "context"

type SymbolSet string

const (
	Runtime SymbolSet = "runtime"
	Test    SymbolSet = "test"
)

type BuildContext struct {
	GOOS, GOARCH string
	Cgo          bool
	CgoSet       bool
	Tags         []string
}
type Options struct {
	Dir       string
	SymbolSet SymbolSet
	Build     BuildContext
	MaxDepth  int
}
type Status string

const (
	Resolved   Status = "resolved"
	External   Status = "external"
	Unknown    Status = "unknown"
	Unresolved Status = "unresolved"
)

type CompatibilityStatus string

const (
	Compatible           CompatibilityStatus = "compatible"
	Incompatible         CompatibilityStatus = "incompatible"
	CompatibilityUnknown CompatibilityStatus = "unknown"
)

type Issue struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}
type Resolution struct {
	Status Status `json:"status"`
	Kind   string `json:"kind,omitempty"`
}
type Compatibility struct {
	Status CompatibilityStatus `json:"status"`
	Issues []Issue             `json:"issues"`
}
type Location struct {
	File string
	Line int
}
type Module struct {
	Path, Dir, Series string
	Requires          map[string]string
	Replaces          map[string][]Replacement
}
type Replacement struct {
	OldVersion string
	NewPath    string
	NewVersion string
}
type Source struct {
	ImportPaths                map[string]string
	PackageNames               map[string]string
	Path, Package, PackageName string
	Build                      BuildContext
	Module                     int
	Test                       bool
}

// Package は対象ファイルをモジュール、ディレクトリ、テスト種別ごとにまとめる。
// Path は既存のシンボルパスを保持するため、一意とは限らない。
// bar の外部テストパッケージと通常の bar_test ディレクトリは、どちらも module/bar_test になり得る。
type Package struct {
	Path, Name, Dir string
	Module          int
	ExternalTest    bool
	Sources         []Source
}
type Workspace struct {
	Modules  []Module
	Packages []Package
	Sources  []Source
}

// TypeRef は名前付き Go 型に正規のパッケージパスを用いる（例: example.com/p.Item）。
// 名前が空の場合は情報不足を示し、型の不適合を意味しない。
type TypeRef struct {
	Name      string
	Value     string
	FieldBase *TypeRef
	FieldName string
}
type Parameter struct {
	Name string
	Type TypeRef
}
type Function struct {
	Main                        bool
	ID, Package, Name, Receiver string
	Params, Results             []Parameter
	Variadic                    bool
	Location                    Location
	Module                      int
	Calls                       []Call
}
type Type struct {
	Embedded       []TypeRef
	Module         int
	ID, Underlying string
	Fields         map[string]TypeRef
	Methods        map[string]Signature
	Alias          bool
}
type Signature struct {
	Params, Results []Parameter
	Variadic        bool
}
type Call struct {
	ReceiverRef                     *TypeRef
	MethodExpression                bool
	Caller, Package, Name, Receiver string
	Arguments                       []TypeRef
	ExpectedResults                 []TypeRef
	ResultCount                     int
	CheckResults                    bool
	Spread                          bool
	Location                        Location
	ExternalKind                    string
	Signature                       *Signature
	Indirect                        bool
}
type SourceModel struct {
	Functions []Function
	Types     []Type
	Imports   map[string]string
}
type Edge struct {
	Caller        string        `json:"caller"`
	Callee        string        `json:"callee"`
	Resolution    Resolution    `json:"resolution"`
	Compatibility Compatibility `json:"compatibility"`
	Location      Location      `json:"-"`
}
type Node struct {
	Name    string  `json:"name"`
	Callers []*Node `json:"callers"`
	Edge    *Edge   `json:"-"`
	Main    bool    `json:"-"`
	Cycle   bool    `json:"-"`
}
type Statistics struct{ DiscoveredSources, AnalyzedSources, LocatorLookups, ResolutionLookups, CacheHits int }
type Result struct {
	Root  *Node
	Edges []Edge
	Stats Statistics
}
type TraversalPolicy struct{ ContinueIncompatible bool }

func (p TraversalPolicy) Continue(c Compatibility) bool {
	return c.Status != Incompatible || p.ContinueIncompatible
}

// Locator はソースの発見時にトークンを走査する場合があるが、詳細な変換は AnalyzeSource で行う。
type Locator interface {
	Definitions(packagePath, name string) []Source
	Callers(packagePath, name string) []Source
}

// Analyze はアプリケーションのエントリーポイントであり、呼び出し側で CLI を起動する必要はない。
func Analyze(ctx context.Context, target string, options Options) (*Result, error) {
	return AnalyzeWithPolicy(ctx, target, options, TraversalPolicy{})
}
