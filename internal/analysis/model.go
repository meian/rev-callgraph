// Package analysis implements source-based reverse call analysis. Library-specific
// syntax trees never cross the source analysis boundary.
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
type Workspace struct {
	Modules []Module
	Sources []Source
}

// TypeRef uses canonical package paths for named Go types, e.g. example.com/p.Item.
// An empty name means insufficient information, never an incompatible type.
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

// Source discovery may scan tokens, but detailed conversion remains in AnalyzeSource.
type Locator interface {
	Definitions(packagePath, name string) []Source
	Callers(packagePath, name string) []Source
}

// Analyze is the application entry point; callers need not start the CLI.
func Analyze(ctx context.Context, target string, options Options) (*Result, error) {
	return AnalyzeWithPolicy(ctx, target, options, TraversalPolicy{})
}
