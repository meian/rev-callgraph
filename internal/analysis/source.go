package analysis

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/constant"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// AnalyzeSource converts one file to source-independent declarations and call sites.
// It deliberately avoids package-wide type checking: mismatched module versions must
// not prevent the rest of a file from being analyzed.
func AnalyzeSource(source Source) (SourceModel, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, source.Path, nil, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return SourceModel{}, fmt.Errorf("parse %s: %w", source.Path, err)
	}
	a := &sourceAnalyzer{source: source, fset: fset, file: file, imports: make(map[string]string), cgo: make(map[string]cDeclaration), functions: make(map[string]Signature), types: make(map[string]TypeRef), fields: make(map[string]map[string]TypeRef), constants: make(map[string]ast.Expr), globalFuncs: make(map[string]Call)}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if replacement := source.ImportPaths[path]; replacement != "" {
			path = replacement
		}
		name := filepath.Base(path)
		if declared := source.PackageNames[path]; declared != "" {
			name = declared
		}
		if imp.Name != nil {
			name = imp.Name.Name
		}
		a.imports[name] = path
		if path == "C" {
			comments := imp.Doc
			for _, decl := range file.Decls {
				if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
					for _, spec := range gen.Specs {
						if spec == imp && comments == nil {
							comments = gen.Doc
						}
					}
				}
			}
			a.loadCDeclarations(comments)
		}
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if i < len(value.Values) {
					a.constants[name.Name] = value.Values[i]
				} else if len(value.Values) == 1 {
					a.constants[name.Name] = value.Values[0]
				}
			}
		}
	}
	model := SourceModel{Imports: a.imports}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						model.Types = append(model.Types, a.analyzeType(ts))
					}
				}
			}
			if d.Tok == token.VAR || d.Tok == token.CONST {
				if a.globals == nil {
					a.globals = make(map[string]TypeRef)
				}
				for _, spec := range d.Specs {
					v, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range v.Names {
						typ := a.typeOf(v.Type)
						if v.Type == nil && i < len(v.Values) {
							typ = defaultType(a.exprType(v.Values[i], &sourceScope{}))
						}
						a.globals[name.Name] = typ
						if i < len(v.Values) {
							if ref, ok := a.functionValue(v.Values[i], &sourceScope{}); ok {
								a.globalFuncs[name.Name] = ref
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			fn := a.function(d)
			model.Functions = append(model.Functions, fn)
			if fn.Receiver == "" {
				a.functions[fn.Name] = Signature{Params: fn.Params, Results: fn.Results, Variadic: fn.Variadic}
			}
		}
	}
	for i, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Body != nil {
			a.analyzeBody(&model.Functions[a.functionIndex(file.Decls, i)], d)
		}
	}
	return model, nil
}

type sourceAnalyzer struct {
	source      Source
	fset        *token.FileSet
	file        *ast.File
	imports     map[string]string
	cgo         map[string]cDeclaration
	functions   map[string]Signature
	types       map[string]TypeRef
	fields      map[string]map[string]TypeRef
	globals     map[string]TypeRef
	globalFuncs map[string]Call
	constants   map[string]ast.Expr
}

func (a *sourceAnalyzer) functionIndex(decls []ast.Decl, index int) int {
	n := 0
	for _, d := range decls[:index] {
		if _, ok := d.(*ast.FuncDecl); ok {
			n++
		}
	}
	return n
}

func (a *sourceAnalyzer) location(pos token.Pos) Location {
	p := a.fset.Position(pos)
	return Location{File: p.Filename, Line: p.Line}
}

func (a *sourceAnalyzer) typeOf(expr ast.Expr) TypeRef {
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "error" || e.Name == "any" || e.Name == "comparable" || builtinType(e.Name) {
			return TypeRef{Name: e.Name}
		}
		if e.Name == "_" {
			return TypeRef{}
		}
		return TypeRef{Name: a.source.Package + "." + e.Name}
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			if path := a.imports[x.Name]; path != "" && path != "C" {
				return TypeRef{Name: path + "." + e.Sel.Name}
			}
		}
	case *ast.StarExpr:
		t := a.typeOf(e.X)
		if t.Name != "" {
			t.Name = "*" + t.Name
		}
		return t
	case *ast.ArrayType:
		t := a.typeOf(e.Elt)
		if t.Name == "" {
			return t
		}
		if e.Len == nil {
			t.Name = "[]" + t.Name
		} else {
			length, ok := a.arrayLength(e.Len, map[string]bool{})
			if !ok || length < 0 {
				return TypeRef{}
			}
			t.Name = "[" + strconv.FormatInt(length, 10) + "]" + t.Name
		}
		return t
	case *ast.Ellipsis:
		t := a.typeOf(e.Elt)
		if t.Name != "" {
			t.Name = "[]" + t.Name
		}
		return t
	case *ast.MapType:
		k, v := a.typeOf(e.Key), a.typeOf(e.Value)
		if k.Name != "" && v.Name != "" {
			return TypeRef{Name: "map[" + k.Name + "]" + v.Name}
		}
	case *ast.ChanType:
		t := a.typeOf(e.Value)
		if t.Name != "" {
			t.Name = "chan " + t.Name
		}
		return t
	case *ast.ParenExpr:
		return a.typeOf(e.X)
	case *ast.IndexExpr:
		return a.typeOf(e.X)
	case *ast.IndexListExpr:
		return a.typeOf(e.X)
	}
	return TypeRef{}
}

func (a *sourceAnalyzer) arrayLength(expr ast.Expr, seen map[string]bool) (int64, bool) {
	switch x := expr.(type) {
	case *ast.BasicLit:
		if x.Kind != token.INT {
			return 0, false
		}
		value := constant.MakeFromLiteral(x.Value, token.INT, 0)
		if value.Kind() != constant.Int {
			return 0, false
		}
		return constant.Int64Val(value)
	case *ast.Ident:
		if seen[x.Name] || a.constants[x.Name] == nil {
			return 0, false
		}
		seen[x.Name] = true
		defer delete(seen, x.Name)
		return a.arrayLength(a.constants[x.Name], seen)
	case *ast.ParenExpr:
		return a.arrayLength(x.X, seen)
	case *ast.UnaryExpr:
		value, ok := a.arrayLength(x.X, seen)
		if !ok {
			return 0, false
		}
		switch x.Op {
		case token.ADD:
			return value, true
		case token.SUB:
			return -value, true
		}
	case *ast.BinaryExpr:
		left, leftOK := a.arrayLength(x.X, seen)
		right, rightOK := a.arrayLength(x.Y, seen)
		if !leftOK || !rightOK {
			return 0, false
		}
		switch x.Op {
		case token.ADD, token.SUB, token.MUL, token.QUO:
			if x.Op == token.QUO && right == 0 {
				return 0, false
			}
			value := constant.BinaryOp(constant.MakeInt64(left), x.Op, constant.MakeInt64(right))
			if value.Kind() != constant.Int {
				return 0, false
			}
			return constant.Int64Val(value)
		}
	}
	return 0, false
}

func builtinType(name string) bool {
	switch name {
	case "bool", "byte", "rune", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128":
		return true
	}
	return false
}

func (a *sourceAnalyzer) parameters(list *ast.FieldList) ([]Parameter, bool) {
	if list == nil {
		return nil, false
	}
	var params []Parameter
	variadic := false
	for i, field := range list.List {
		if _, ok := field.Type.(*ast.Ellipsis); ok && i == len(list.List)-1 {
			variadic = true
		}
		t := a.typeOf(field.Type)
		if len(field.Names) == 0 {
			params = append(params, Parameter{Type: t})
			continue
		}
		for _, n := range field.Names {
			params = append(params, Parameter{Name: n.Name, Type: t})
		}
	}
	return params, variadic
}

func (a *sourceAnalyzer) signature(ft *ast.FuncType) Signature {
	p, v := a.parameters(ft.Params)
	r, _ := a.parameters(ft.Results)
	return Signature{Params: p, Results: r, Variadic: v}
}

func (a *sourceAnalyzer) function(d *ast.FuncDecl) Function {
	sig := a.signature(d.Type)
	fn := Function{Package: a.source.Package, Name: d.Name.Name, Module: a.source.Module, Params: sig.Params, Results: sig.Results, Variadic: sig.Variadic, Location: a.location(d.Pos()), Main: a.source.PackageName == "main"}
	fn.ID = a.source.Package + "." + fn.Name
	if d.Recv != nil && len(d.Recv.List) != 0 {
		fn.Receiver = a.typeOf(d.Recv.List[0].Type).Name
		base := strings.TrimPrefix(fn.Receiver, "*")
		fn.ID = base + "#" + fn.Name
	}
	return fn
}

func (a *sourceAnalyzer) analyzeType(ts *ast.TypeSpec) Type {
	t := Type{Module: a.source.Module, ID: a.source.Package + "." + ts.Name.Name, Fields: make(map[string]TypeRef), Methods: make(map[string]Signature), Alias: ts.Assign.IsValid()}
	a.types[ts.Name.Name] = TypeRef{Name: t.ID}
	if alias := a.typeOf(ts.Type); alias.Name != "" {
		t.Underlying = alias.Name
	}
	switch x := ts.Type.(type) {
	case *ast.StructType:
		t.Underlying = "struct"
		for _, f := range x.Fields.List {
			if len(f.Names) == 0 {
				ref := a.typeOf(f.Type)
				t.Embedded = append(t.Embedded, ref)
				base := strings.TrimPrefix(ref.Name, "*")
				if i := strings.LastIndex(base, "."); i >= 0 {
					base = base[i+1:]
				}
				t.Fields[base] = ref
			}
			for _, n := range f.Names {
				t.Fields[n.Name] = a.typeOf(f.Type)
			}
		}
		a.fields[t.ID] = t.Fields
	case *ast.InterfaceType:
		t.Underlying = "interface"
		for _, f := range x.Methods.List {
			if len(f.Names) == 0 {
				t.Embedded = append(t.Embedded, a.typeOf(f.Type))
			}
			if ft, ok := f.Type.(*ast.FuncType); ok {
				for _, n := range f.Names {
					t.Methods[n.Name] = a.signature(ft)
				}
			}
		}
	}
	return t
}

type sourceScope struct {
	vars     map[string]TypeRef
	funcs    map[string]Call
	declared map[string]bool
}

func cloneScope(scope sourceScope) sourceScope {
	child := sourceScope{vars: make(map[string]TypeRef, len(scope.vars)), funcs: make(map[string]Call, len(scope.funcs)), declared: make(map[string]bool)}
	for k, v := range scope.vars {
		child.vars[k] = v
	}
	for k, v := range scope.funcs {
		child.funcs[k] = v
	}
	return child
}

func startsSourceScope(n ast.Node) bool {
	switch n.(type) {
	case *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt, *ast.SwitchStmt,
		*ast.TypeSwitchStmt, *ast.RangeStmt, *ast.SelectStmt,
		*ast.CaseClause, *ast.CommClause:
		return true
	}
	return false
}

func (a *sourceAnalyzer) analyzeBody(fn *Function, d *ast.FuncDecl) {
	scope := sourceScope{vars: make(map[string]TypeRef), funcs: make(map[string]Call), declared: make(map[string]bool)}
	for name, typ := range a.globals {
		scope.vars[name] = typ
	}
	for name, ref := range a.globalFuncs {
		scope.funcs[name] = ref
	}
	for _, p := range fn.Params {
		if p.Name != "" {
			scope.vars[p.Name] = p.Type
			scope.declared[p.Name] = true
			delete(scope.funcs, p.Name)
		}
	}
	for _, p := range fn.Results {
		if p.Name != "" {
			scope.vars[p.Name] = p.Type
			scope.declared[p.Name] = true
			delete(scope.funcs, p.Name)
		}
	}
	if d.Recv != nil && len(d.Recv.List) != 0 {
		for _, n := range d.Recv.List[0].Names {
			scope.vars[n.Name] = TypeRef{Name: fn.Receiver}
			scope.declared[n.Name] = true
			delete(scope.funcs, n.Name)
		}
	}
	var stack []ast.Node
	var scopes []sourceScope
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if n == nil {
			if startsSourceScope(stack[len(stack)-1]) {
				scope = scopes[len(scopes)-1]
				scopes = scopes[:len(scopes)-1]
			}
			stack = stack[:len(stack)-1]
			return true
		}
		var parent ast.Node
		if len(stack) != 0 {
			parent = stack[len(stack)-1]
		}
		stack = append(stack, n)
		if startsSourceScope(n) {
			scopes = append(scopes, scope)
			scope = cloneScope(scope)
		}
		switch x := n.(type) {
		case *ast.BlockStmt:
			if x == d.Body {
				for k, v := range scopes[len(scopes)-1].declared {
					scope.declared[k] = v
				}
			}
		case *ast.ValueSpec:
			for i, name := range x.Names {
				t := a.typeOf(x.Type)
				if x.Type == nil && i < len(x.Values) {
					t = defaultType(a.exprType(x.Values[i], &scope))
				}
				scope.vars[name.Name] = t
				scope.declared[name.Name] = true
				if i < len(x.Values) {
					if ref, ok := a.functionValue(x.Values[i], &scope); ok {
						scope.funcs[name.Name] = ref
					} else {
						delete(scope.funcs, name.Name)
					}
				} else {
					delete(scope.funcs, name.Name)
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				if i < len(x.Rhs) {
					if x.Tok == token.DEFINE && !scope.declared[id.Name] {
						scope.vars[id.Name] = defaultType(a.exprType(x.Rhs[i], &scope))
						scope.declared[id.Name] = true
					}
					if ref, ok := a.functionValue(x.Rhs[i], &scope); ok {
						scope.funcs[id.Name] = ref
					} else {
						delete(scope.funcs, id.Name)
					}
				}
			}
		case *ast.RangeStmt:
			// A range element needs its iterable's exact type. Keep it unknown.
			for _, target := range []ast.Expr{x.Key, x.Value} {
				if id, ok := target.(*ast.Ident); ok {
					scope.vars[id.Name] = TypeRef{}
				}
			}
		case *ast.CallExpr:
			if a.isConversionOrBuiltin(x, &scope) {
				break
			}
			call := a.call(x, fn.ID, &scope)
			a.resultContext(&call, x, parent, fn, &scope)
			fn.Calls = append(fn.Calls, call)
		}
		return true
	})
}

func (a *sourceAnalyzer) isConversionOrBuiltin(expr *ast.CallExpr, scope *sourceScope) bool {
	id, ok := expr.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	if _, shadow := scope.vars[id.Name]; shadow {
		return false
	}
	if builtinType(id.Name) || a.types[id.Name].Name != "" {
		return true
	}
	switch id.Name {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover":
		return true
	}
	return false
}

func (a *sourceAnalyzer) functionValue(expr ast.Expr, scope *sourceScope) (Call, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		if ref, ok := scope.funcs[x.Name]; ok {
			return ref, true
		}
		if _, shadowed := scope.vars[x.Name]; !shadowed && !builtinType(x.Name) && a.types[x.Name].Name == "" {
			path := a.source.Package
			if _, local := a.functions[x.Name]; !local {
				if dot := a.imports["."]; dot != "" {
					path = dot
				}
			}
			return Call{Package: path, Name: x.Name}, true
		}
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			if path := a.importPath(id.Name, scope); path != "" {
				return Call{Package: path, Name: x.Sel.Name}, true
			}
		}
		if recv := a.methodExpressionReceiver(x.X, scope); recv.Name != "" {
			return Call{Package: packageFromType(recv.Name), Receiver: recv.Name, Name: x.Sel.Name, MethodExpression: true}, true
		}
		if recv := a.exprType(x.X, scope).Name; recv != "" && recv != "any" {
			return Call{Package: packageFromType(recv), Receiver: recv, Name: x.Sel.Name}, true
		}
	}
	return Call{}, false
}

func (a *sourceAnalyzer) call(expr *ast.CallExpr, caller string, scope *sourceScope) Call {
	c := Call{Caller: caller, Location: a.location(expr.Pos()), Spread: expr.Ellipsis.IsValid(), Indirect: true}
	for _, arg := range expr.Args {
		c.Arguments = append(c.Arguments, a.exprType(arg, scope))
	}
	fu := expr.Fun
	for {
		switch indexed := fu.(type) {
		case *ast.IndexExpr:
			fu = indexed.X
		case *ast.IndexListExpr:
			fu = indexed.X
		default:
			goto unwrapped
		}
	}
unwrapped:
	switch f := fu.(type) {
	case *ast.Ident:
		if ref, ok := scope.funcs[f.Name]; ok {
			c.Package, c.Name, c.Receiver, c.Indirect, c.MethodExpression = ref.Package, ref.Name, ref.Receiver, false, ref.MethodExpression
		} else if _, shadow := scope.vars[f.Name]; !shadow && !builtinType(f.Name) {
			path := a.source.Package
			if _, local := a.functions[f.Name]; !local {
				if dot := a.imports["."]; dot != "" {
					path = dot
				}
			}
			c.Package, c.Name, c.Indirect = path, f.Name, false
		}
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			if path := a.importPath(id.Name, scope); path != "" {
				c.Package, c.Name, c.Indirect = path, f.Sel.Name, false
				if path == "C" {
					if decl, ok := a.cgo[f.Sel.Name]; ok {
						c.ExternalKind, c.Signature = decl.kind, &decl.signature
					}
				}
				break
			}
		}
		if recv := a.methodExpressionReceiver(f.X, scope); recv.Name != "" {
			c.Receiver, c.Name, c.Indirect, c.MethodExpression = recv.Name, f.Sel.Name, false, true
			c.Package = packageFromType(recv.Name)
			break
		}
		recvRef := a.exprType(f.X, scope)
		recv := recvRef.Name
		if recvRef.FieldBase != nil {
			c.ReceiverRef = &recvRef
			c.Name = f.Sel.Name
			c.Indirect = false
		}
		if recv != "" && recv != "any" && recv != "interface{}" {
			c.Receiver, c.Name, c.Indirect = recv, f.Sel.Name, false
			c.Package = packageFromType(recv)
		}
	}
	return c
}

func (a *sourceAnalyzer) methodExpressionReceiver(expr ast.Expr, scope *sourceScope) TypeRef {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return a.methodExpressionReceiver(x.X, scope)
	case *ast.StarExpr:
		t := a.methodExpressionReceiver(x.X, scope)
		if t.Name != "" {
			t.Name = "*" + t.Name
		}
		return t
	case *ast.Ident:
		if _, shadowed := scope.vars[x.Name]; shadowed {
			return TypeRef{}
		}
		if typ := a.types[x.Name]; typ.Name != "" {
			return typ
		}
		if !builtinType(x.Name) {
			return TypeRef{Name: a.source.Package + "." + x.Name}
		}
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && ast.IsExported(x.Sel.Name) {
			if path := a.importPath(id.Name, scope); path != "" && path != "C" {
				return TypeRef{Name: path + "." + x.Sel.Name}
			}
		}
	}
	return TypeRef{}
}

func (a *sourceAnalyzer) importPath(name string, scope *sourceScope) string {
	if scope != nil {
		if _, shadowed := scope.vars[name]; shadowed {
			return ""
		}
	}
	return a.imports[name]
}

func packageFromType(name string) string {
	name = strings.TrimPrefix(name, "*")
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return ""
}

func (a *sourceAnalyzer) exprType(expr ast.Expr, scope *sourceScope) TypeRef {
	switch x := expr.(type) {
	case *ast.BasicLit:
		switch x.Kind {
		case token.STRING:
			return TypeRef{Name: "untyped string", Value: x.Value}
		case token.CHAR:
			return TypeRef{Name: "untyped rune", Value: x.Value}
		case token.INT:
			return TypeRef{Name: "untyped int", Value: x.Value}
		case token.FLOAT:
			return TypeRef{Name: "untyped float", Value: x.Value}
		case token.IMAG:
			return TypeRef{Name: "untyped complex", Value: x.Value}
		}
	case *ast.Ident:
		if t, ok := scope.vars[x.Name]; ok {
			return t
		}
		if t, ok := a.types[x.Name]; ok {
			return t
		}
		if x.Name == "true" || x.Name == "false" {
			return TypeRef{Name: "untyped bool", Value: x.Name}
		}
		if builtinType(x.Name) {
			return TypeRef{Name: x.Name}
		}
	case *ast.CompositeLit:
		return a.typeOf(x.Type)
	case *ast.UnaryExpr:
		t := a.exprType(x.X, scope)
		if x.Op == token.SUB && t.Value != "" {
			t.Value = "-" + t.Value
		}
		if x.Op == token.AND && t.Name != "" {
			t.Name = "*" + t.Name
		}
		if x.Op == token.MUL {
			t.Name = strings.TrimPrefix(t.Name, "*")
		}
		return t
	case *ast.ParenExpr:
		return a.exprType(x.X, scope)
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			if a.importPath(id.Name, scope) != "" {
				return TypeRef{}
			}
		}
		baseRef := a.exprType(x.X, scope)
		base := strings.TrimPrefix(baseRef.Name, "*")
		if fields := a.fields[base]; fields != nil {
			return fields[x.Sel.Name]
		}
		return TypeRef{FieldBase: &baseRef, FieldName: x.Sel.Name}
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok {
			if builtinType(id.Name) && len(x.Args) == 1 {
				return TypeRef{Name: id.Name}
			}
			if sig, ok := a.functions[id.Name]; ok && len(sig.Results) == 1 {
				return sig.Results[0].Type
			}
		}
	case *ast.IndexExpr:
		// The indexed type may be a map, slice or generic instantiation.
		return TypeRef{}
	}
	return TypeRef{}
}

func defaultType(t TypeRef) TypeRef {
	switch t.Name {
	case "untyped string":
		t.Name = "string"
	case "untyped rune":
		t.Name = "rune"
	case "untyped int":
		t.Name = "int"
	case "untyped float":
		t.Name = "float64"
	case "untyped complex":
		t.Name = "complex128"
	case "untyped bool":
		t.Name = "bool"
	}
	return t
}

// AnalyzeExternal reads the named declaration using the host build context.
func AnalyzeExternal(packagePath, name string) (*Signature, error) {
	return AnalyzeExternalContext(packagePath, name, BuildContext{})
}

// AnalyzeExternalContext reads only files selected by the target build context
// from a Go standard-library package. A missing symbol returns nil.
func AnalyzeExternalContext(packagePath, name string, buildContext BuildContext) (*Signature, error) {
	function, err := AnalyzeExternalFunction(packagePath, name, "", buildContext)
	if err != nil || function == nil {
		return nil, err
	}
	return &Signature{Params: function.Params, Results: function.Results, Variadic: function.Variadic}, nil
}

// AnalyzeExternalFunction identifies a standard-library function or a method
// of the specified receiver, preserving its declared receiver for method expressions.
// receiver is a canonical type name (optionally prefixed with *); empty selects
// package-level functions only.
func AnalyzeExternalFunction(packagePath, name, receiver string, buildContext BuildContext) (*Function, error) {
	if packagePath == "" || packagePath == "C" {
		return nil, nil
	}
	buildConfig := build.Default
	if buildContext.GOOS != "" {
		buildConfig.GOOS = buildContext.GOOS
	}
	if buildContext.GOARCH != "" {
		buildConfig.GOARCH = buildContext.GOARCH
	}
	if buildContext.CgoSet {
		buildConfig.CgoEnabled = buildContext.Cgo
	} else if buildConfig.GOOS != build.Default.GOOS || buildConfig.GOARCH != build.Default.GOARCH {
		buildConfig.CgoEnabled = false
	}
	if len(buildContext.Tags) != 0 {
		buildConfig.BuildTags = append([]string(nil), buildContext.Tags...)
	}
	pkg, err := buildConfig.Import(packagePath, "", 0)
	if err != nil {
		return nil, nil
	}
	root := filepath.Join(buildConfig.GOROOT, "src")
	rel, err := filepath.Rel(root, pkg.Dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, nil
	}
	files := append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...)
	for _, fileName := range files {
		path := filepath.Join(pkg.Dir, fileName)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse external %s: %w", path, err)
		}
		a := &sourceAnalyzer{source: Source{Package: packagePath}, imports: make(map[string]string), fset: fset}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			alias := filepath.Base(path)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			a.imports[alias] = path
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != name {
				continue
			}
			if receiver == "" {
				if fn.Recv != nil {
					continue
				}
			} else {
				if fn.Recv == nil || len(fn.Recv.List) != 1 {
					continue
				}
				declared := a.typeOf(fn.Recv.List[0].Type).Name
				if strings.TrimPrefix(declared, "*") != strings.TrimPrefix(receiver, "*") {
					continue
				}
			}
			function := a.function(fn)
			return &function, nil
		}
	}
	return nil, nil
}

func (a *sourceAnalyzer) resultContext(c *Call, expr *ast.CallExpr, parent ast.Node, fn *Function, scope *sourceScope) {
	switch p := parent.(type) {
	case *ast.AssignStmt:
		if len(p.Rhs) == 1 && p.Rhs[0] == expr {
			c.CheckResults, c.ResultCount = true, len(p.Lhs)
			for _, lhs := range p.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					c.ExpectedResults = append(c.ExpectedResults, scope.vars[id.Name])
				} else {
					c.ExpectedResults = append(c.ExpectedResults, TypeRef{})
				}
			}
			return
		}
		if len(p.Rhs) == len(p.Lhs) {
			for i, rhs := range p.Rhs {
				if rhs == expr {
					c.CheckResults, c.ResultCount = true, 1
					if id, ok := p.Lhs[i].(*ast.Ident); ok {
						c.ExpectedResults = []TypeRef{scope.vars[id.Name]}
					}
					return
				}
			}
		}
	case *ast.ReturnStmt:
		if len(p.Results) == 1 && p.Results[0] == expr {
			c.CheckResults, c.ResultCount = true, len(fn.Results)
			for _, r := range fn.Results {
				c.ExpectedResults = append(c.ExpectedResults, r.Type)
			}
			return
		}
		if len(p.Results) == len(fn.Results) {
			for i, r := range p.Results {
				if r == expr {
					c.CheckResults, c.ResultCount = true, 1
					c.ExpectedResults = []TypeRef{fn.Results[i].Type}
					return
				}
			}
		}
	case *ast.ValueSpec:
		if len(p.Values) == 1 && p.Values[0] == expr {
			c.CheckResults, c.ResultCount = true, len(p.Names)
			for range p.Names {
				c.ExpectedResults = append(c.ExpectedResults, a.typeOf(p.Type))
			}
			return
		}
	case *ast.ExprStmt:
		return
	}
}

type cDeclaration struct {
	signature Signature
	kind      string
}

var (
	cInclude  = regexp.MustCompile(`(?m)^\s*#\s*include\s*"([^"]+)"`)
	cFunction = regexp.MustCompile(`(?m)(?:^|[;}\n])\s*(?:extern\s+)?([A-Za-z_][\w\s\*]*?)\s+([A-Za-z_]\w*)\s*\(([^()]*)\)\s*(?:;|\{)`)
)

func (a *sourceAnalyzer) loadCDeclarations(comments *ast.CommentGroup) {
	if comments == nil {
		return
	}
	var preamble strings.Builder
	for _, c := range comments.List {
		preamble.WriteString(strings.TrimSuffix(strings.TrimPrefix(c.Text, "/*"), "*/"))
		preamble.WriteByte('\n')
	}
	a.parseCDeclarations(preamble.String(), "cgo-preamble")
	for _, match := range cInclude.FindAllStringSubmatch(preamble.String(), -1) {
		name := filepath.Clean(match[1])
		if filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(a.source.Path), name))
		if err == nil {
			a.parseCDeclarations(string(data), "cgo-header")
		}
	}
}

func (a *sourceAnalyzer) parseCDeclarations(content, kind string) {
	for _, match := range cFunction.FindAllStringSubmatch(content, -1) {
		name := match[2]
		if strings.Contains(match[1], "typedef") || strings.HasPrefix(name, "if") {
			continue
		}
		var sig Signature
		params := strings.TrimSpace(match[3])
		if params != "" && params != "void" {
			for _, part := range strings.Split(params, ",") {
				sig.Params = append(sig.Params, Parameter{Type: cType(part)})
			}
		}
		if strings.TrimSpace(match[1]) != "void" {
			sig.Results = []Parameter{{Type: cType(match[1])}}
		}
		a.cgo[name] = cDeclaration{signature: sig, kind: kind}
	}
}

func cType(s string) TypeRef {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "char") && strings.Contains(s, "*") {
		return TypeRef{Name: "*C.char"}
	}
	for _, typ := range []string{"double", "float", "long", "short", "int", "char"} {
		if strings.Contains(s, typ) {
			return TypeRef{Name: typ}
		}
	}
	return TypeRef{}
}
