package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Target is the public CLI symbol grammar, independent of source syntax.
type Target struct{ Package, Receiver, Name string }

func ParseTarget(value string) (Target, error) {
	var t Target
	i := strings.LastIndex(value, ".")
	if i < 1 || i == len(value)-1 {
		return t, fmt.Errorf("invalid target: %s", value)
	}
	t.Package = value[:i]
	tail := value[i+1:]
	if strings.Contains(tail, "#") {
		p := strings.Split(tail, "#")
		if len(p) != 2 || p[0] == "" || p[1] == "" {
			return Target{}, fmt.Errorf("invalid target: %s", value)
		}
		t.Receiver = p[0]
		t.Name = p[1]
	} else {
		t.Name = tail
	}
	if strings.ContainsAny(t.Name, " /#") || strings.ContainsAny(t.Receiver, " /#") {
		return Target{}, fmt.Errorf("invalid target: %s", value)
	}
	return t, nil
}
func (t Target) ID() string {
	if t.Receiver != "" {
		return t.Package + "." + t.Receiver + "#" + t.Name
	}
	return t.Package + "." + t.Name
}

type engine struct {
	ctx             context.Context
	workspace       *Workspace
	locator         Locator
	models          map[string]SourceModel
	functions       map[string]Function
	types           map[string]Type
	stats           Statistics
	callerCache     map[string][]Edge
	definitionCache map[string]bool
	externalCache   map[string]*Signature
	build           BuildContext
}

func AnalyzeWithPolicy(ctx context.Context, target string, options Options, policy TraversalPolicy) (*Result, error) {
	t, err := ParseTarget(target)
	if err != nil {
		return nil, err
	}
	w, err := Discover(ctx, options)
	if err != nil {
		return nil, err
	}
	locator, err := NewLocator(w)
	if err != nil {
		return nil, err
	}
	e := &engine{ctx: ctx, workspace: w, locator: locator, models: map[string]SourceModel{}, functions: map[string]Function{}, types: map[string]Type{}, callerCache: map[string][]Edge{}, definitionCache: map[string]bool{}, externalCache: map[string]*Signature{}, build: options.Build}
	e.stats.DiscoveredSources = len(w.Sources)
	// Missing symbols remain valid roots so callers of a removed API can be shown.
	if err = e.definitions(t.Package, t.Name); err != nil {
		return nil, err
	}
	root := &Node{Name: t.ID(), Main: e.functions[t.ID()].Main, Callers: []*Node{}}
	result := &Result{Root: root, Edges: []Edge{}}
	seenEdges := map[string]bool{}
	var visit func(*Node, int, map[string]bool) error
	visit = func(node *Node, depth int, ancestors map[string]bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if options.MaxDepth > 0 && depth >= options.MaxDepth {
			return nil
		}
		callers, err := e.callersFor(node.Name)
		if err != nil {
			return err
		}
		// Merge only equivalent outcomes. A failing call site must not hide a
		// separate compatible route through the same caller.
		unique := map[string]bool{}
		for _, edge := range callers {
			key := edgeOutcomeKey(edge)
			if unique[key] {
				continue
			}
			unique[key] = true
			name := edge.Caller
			if !seenEdges[key] {
				result.Edges = append(result.Edges, edge)
				seenEdges[key] = true
			}
			child := &Node{Name: name, Edge: &edge, Callers: []*Node{}, Main: e.functions[name].Main, Cycle: ancestors[name]}
			node.Callers = append(node.Callers, child)
			if child.Cycle || !policy.Continue(edge.Compatibility) || edge.Resolution.Status == External {
				continue
			}
			ancestors[name] = true
			if err = visit(child, depth+1, ancestors); err != nil {
				return err
			}
			delete(ancestors, name)
		}
		return nil
	}
	if err = visit(root, 0, map[string]bool{root.Name: true}); err != nil {
		return nil, err
	}
	result.Stats = e.stats
	return result, nil
}
func (e *engine) load(s Source) error {
	if _, ok := e.models[s.Path]; ok {
		e.stats.CacheHits++
		return nil
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	model, err := AnalyzeSource(s)
	if err != nil {
		return fmt.Errorf("analyze %s: %w", s.Path, err)
	}
	e.models[s.Path] = model
	e.stats.AnalyzedSources++
	for _, f := range model.Functions {
		e.functions[f.ID] = f
	}
	for _, t := range model.Types {
		e.types[t.ID] = t
	}
	return nil
}
func (e *engine) definitions(pkg, name string) error {
	key := pkg + "\x00" + name
	if e.definitionCache[key] {
		e.stats.CacheHits++
		return nil
	}
	e.definitionCache[key] = true
	e.stats.LocatorLookups++
	for _, s := range e.locator.Definitions(pkg, name) {
		if err := e.load(s); err != nil {
			return err
		}
	}
	return nil
}
func receiverID(receiver string) string { return strings.TrimPrefix(receiver, "*") }
func (e *engine) resolve(caller Function, call Call) (string, Resolution, *Function, error) {
	e.stats.ResolutionLookups++
	if call.ReceiverRef != nil {
		call.Receiver = e.resolveType(*call.ReceiverRef, map[string]bool{}).Name
		if call.Receiver == "" {
			return "", Resolution{Status: Unknown}, nil, nil
		}
	}
	pkg, name := call.Package, call.Name
	if pkg == "" {
		pkg = caller.Package
	}
	id := pkg + "." + name
	if call.Receiver != "" {
		id = receiverID(call.Receiver) + "#" + name
		if i := strings.LastIndex(receiverID(call.Receiver), "."); i >= 0 {
			pkg = receiverID(call.Receiver)[:i]
		}
	}
	if call.Indirect && call.Receiver == "" {
		return "", Resolution{Status: Unknown}, nil, nil
	}
	if err := e.definitions(pkg, name); err != nil {
		return "", Resolution{}, nil, err
	}
	if f, ok := e.functions[id]; ok {
		if !e.sameSeries(caller.Module, f.Module) {
			return "", Resolution{Status: External, Kind: "different-module-series"}, nil, nil
		}
		return id, Resolution{Status: Resolved}, &f, nil
	}
	// Resolve methods promoted through embedded fields, without treating a
	// similarly named method on another concrete receiver as the same symbol.
	if call.Receiver != "" {
		if f, ok, err := e.method(receiverID(call.Receiver), name, map[string]bool{}); err != nil {
			return "", Resolution{}, nil, err
		} else if ok {
			if !e.sameSeries(caller.Module, f.Module) {
				return "", Resolution{Status: External, Kind: "different-module-series"}, nil, nil
			}
			return f.ID, Resolution{Status: Resolved}, &f, nil
		}
	}
	if call.ExternalKind != "" {
		return id, Resolution{Status: External, Kind: call.ExternalKind}, nil, nil
	}
	for _, s := range e.workspace.Sources {
		if s.Package == pkg {
			if !e.sameSeries(caller.Module, s.Module) {
				return "", Resolution{Status: External, Kind: "different-module-series"}, nil, nil
			}
			return id, Resolution{Status: Unknown, Kind: "missing-symbol"}, nil, nil
		}
	}
	// Read a declaration before marking a standard-library symbol external.
	if !strings.Contains(strings.Split(pkg, "/")[0], ".") && pkg != "C" {
		sig, ok := e.externalCache[id]
		if !ok {
			var err error
			sig, err = AnalyzeExternalContext(pkg, name, e.build)
			if err != nil {
				return id, Resolution{Status: Unknown}, nil, nil
			}
			e.externalCache[id] = sig
		} else {
			e.stats.CacheHits++
		}
		if sig != nil {
			f := &Function{ID: id, Package: pkg, Name: name, Params: sig.Params, Results: sig.Results, Variadic: sig.Variadic}
			return id, Resolution{Status: External, Kind: "standard-library"}, f, nil
		}
	}
	// Syntax alone does not establish that a dependency symbol exists.
	return id, Resolution{Status: Unknown, Kind: "definition-unavailable"}, nil, nil
}
func (e *engine) method(receiver, name string, seen map[string]bool) (Function, bool, error) {
	if seen[receiver] {
		return Function{}, false, nil
	}
	seen[receiver] = true
	i := strings.LastIndex(receiver, ".")
	if i < 0 {
		return Function{}, false, nil
	}
	if err := e.definitions(receiver[:i], receiver[i+1:]); err != nil {
		return Function{}, false, err
	}
	typ, ok := e.types[receiver]
	if !ok {
		return Function{}, false, nil
	}
	if signature, ok := typ.Methods[name]; ok {
		return Function{ID: receiver + "#" + name, Package: receiver[:i], Name: name, Receiver: receiver, Module: typ.Module, Params: signature.Params, Results: signature.Results, Variadic: signature.Variadic}, true, nil
	}
	if typ.Alias && typ.Underlying != "" {
		id := receiverID(typ.Underlying) + "#" + name
		if f, ok := e.functions[id]; ok {
			return f, true, nil
		}
		return e.method(receiverID(typ.Underlying), name, seen)
	}
	var found []Function
	for _, t := range typ.Embedded {
		base := receiverID(t.Name)
		if i := strings.LastIndex(base, "."); i > 0 {
			if err := e.definitions(base[:i], name); err != nil {
				return Function{}, false, err
			}
		}
		if f, ok := e.functions[base+"#"+name]; ok {
			found = append(found, f)
		} else if f, ok, err := e.method(base, name, seen); err != nil {
			return Function{}, false, err
		} else if ok {
			found = append(found, f)
		}
	}
	if len(found) == 1 {
		return found[0], true, nil
	}
	return Function{}, false, nil
}
func (e *engine) sameSeries(from, to int) bool {
	if from == to {
		return true
	}
	if from < 0 || to < 0 || from >= len(e.workspace.Modules) || to >= len(e.workspace.Modules) {
		return false
	}
	a, b := e.workspace.Modules[from], e.workspace.Modules[to]
	for required, version := range a.Requires {
		if requireTargetsModule(a, required, b) {
			if replacement, ok := selectedReplacement(a, required); ok && replacement.NewVersion != "" {
				version = replacement.NewVersion
			}
			major := versionMajor(version)
			return major != "" && b.Series != "" && major == b.Series
		}
	}
	// Explicit replacement without a require also identifies a local source.
	for required := range a.Replaces {
		if requireTargetsModule(a, required, b) {
			return b.Series != ""
		}
	}
	return b.Series != ""
}
func versionMajor(v string) string {
	if !strings.HasPrefix(v, "v") {
		return ""
	}
	if i := strings.IndexByte(v, '.'); i > 1 {
		return v[:i]
	}
	return ""
}

func (e *engine) resolveType(ref TypeRef, seen map[string]bool) TypeRef {
	if ref.FieldBase == nil {
		return ref
	}
	base := e.resolveType(*ref.FieldBase, seen)
	name := receiverID(base.Name)
	if name == "" || seen[name] {
		return TypeRef{}
	}
	seen[name] = true
	if i := strings.LastIndex(name, "."); i > 0 {
		_ = e.definitions(name[:i], name[i+1:])
	}
	typ, ok := e.types[name]
	if !ok {
		return TypeRef{}
	}
	if field, ok := typ.Fields[ref.FieldName]; ok {
		return e.resolveType(field, seen)
	}
	if typ.Alias {
		return e.resolveType(TypeRef{FieldBase: &TypeRef{Name: typ.Underlying}, FieldName: ref.FieldName}, seen)
	}
	return TypeRef{}
}

// callersFor resolves source candidates once per symbol and keeps traversal
// independent of discovery, conversion, and compatibility checking.
func (e *engine) callersFor(targetID string) ([]Edge, error) {
	symbol, err := ParseTarget(targetID)
	if err != nil {
		return nil, err
	}
	callers, cached := e.callerCache[targetID]
	if !cached {
		e.stats.LocatorLookups++
		for _, source := range e.locator.Callers(symbol.Package, symbol.Name) {
			if err = e.load(source); err != nil {
				return nil, err
			}
		}
		// Loading definitions may extend the function index; use a stable snapshot.
		funcs := make([]Function, 0, len(e.functions))
		for _, f := range e.functions {
			funcs = append(funcs, f)
		}
		for _, f := range funcs {
			for _, call := range f.Calls {
				// A source model already records the referenced symbol name,
				// including resolved function-value aliases. Do not load other
				// callees merely because they share this source file.
				if call.Name != symbol.Name {
					continue
				}
				id, res, callee, err := e.resolve(f, call)
				if err != nil {
					return nil, err
				}
				if id != targetID {
					continue
				}
				comp := e.compatibility(call, callee, res)
				callers = append(callers, Edge{Caller: f.ID, Callee: id, Resolution: res, Compatibility: comp, Location: call.Location})
			}
		}

		e.callerCache[targetID] = callers
	} else {
		e.stats.CacheHits++
	}
	sort.Slice(callers, func(i, j int) bool {
		if callers[i].Caller != callers[j].Caller {
			return callers[i].Caller < callers[j].Caller
		}
		if callers[i].Location.File != callers[j].Location.File {
			return callers[i].Location.File < callers[j].Location.File
		}
		return callers[i].Location.Line < callers[j].Location.Line
	})

	return callers, nil
}

// edgeOutcomeKey preserves independent compatibility outcomes while retaining
// one graph branch for repeated equivalent calls, as in the original output.
func edgeOutcomeKey(edge Edge) string {
	data, _ := json.Marshal(struct {
		Caller        string
		Callee        string
		Resolution    Resolution
		Compatibility Compatibility
	}{edge.Caller, edge.Callee, edge.Resolution, edge.Compatibility})
	return string(data)
}
