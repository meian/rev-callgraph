package analysis

import (
	"go/constant"
	"go/token"
	"runtime"
	"strconv"
	"strings"
)

func (e *engine) compatibility(call Call, callee *Function, res Resolution) Compatibility {
	c := Compatibility{Status: Compatible, Issues: []Issue{}}
	issue := func(kind, message string) {
		c.Status = Incompatible
		c.Issues = append(c.Issues, Issue{Kind: kind, Message: message})
	}
	var sig *Signature
	if callee != nil {
		sig = &Signature{Params: callee.Params, Results: callee.Results, Variadic: callee.Variadic}
	} else {
		sig = call.Signature
	}
	if sig == nil {
		if res.Kind == "missing-symbol" {
			issue("missing-symbol", "callee definition is absent from the selected source set")
		} else {
			c.Status = CompatibilityUnknown
		}
		return c
	}
	if call.MethodExpression && callee != nil {
		sig = &Signature{Params: append([]Parameter{{Type: TypeRef{Name: callee.Receiver}}}, sig.Params...), Results: sig.Results, Variadic: sig.Variadic}
	}
	n := len(sig.Params)
	argc := len(call.Arguments)
	if !sig.Variadic && call.Spread {
		issue("variadic", "spread argument supplied to non-variadic function")
	}
	if (!sig.Variadic && argc != n) || (sig.Variadic && argc < n-1) || (sig.Variadic && call.Spread && argc != n) {
		issue("argument-count", "argument count does not match the current signature")
	}
	uncertain := false
	for i, arg := range call.Arguments {
		j := i
		if sig.Variadic && j >= n-1 {
			j = n - 1
		}
		if j < 0 || j >= n {
			continue
		}
		want := sig.Params[j].Type.Name
		if sig.Variadic && j == n-1 {
			want = strings.TrimPrefix(want, "...")
			if call.Spread {
				if !strings.HasPrefix(want, "[]") {
					want = "[]" + want
				}
			} else {
				want = strings.TrimPrefix(want, "[]")
			}
		}
		match, known := e.argumentAssignable(arg, want)
		if !known {
			uncertain = true
		} else if !match {
			issue("argument-type", "argument type "+arg.Name+" is not assignable to "+want)
		}
	}
	if call.CheckResults {
		if call.ResultCount != len(sig.Results) {
			issue("result-count", "result count does not match the current signature")
		}
		for i, want := range call.ExpectedResults {
			if i >= len(sig.Results) {
				break
			}
			if want.Name == "" {
				continue
			}
			match, known := e.assignable(sig.Results[i].Type.Name, want.Name, map[string]bool{})
			if !known {
				uncertain = true
			} else if !match {
				issue("result-type", "result type "+sig.Results[i].Type.Name+" is not assignable to "+want.Name)
			}
		}
	}
	if c.Status != Incompatible && uncertain {
		c.Status = CompatibilityUnknown
	}
	return c
}
func (e *engine) assignable(from, to string, seen map[string]bool) (bool, bool) {
	if from == "" || to == "" {
		return false, false
	}
	if from == to {
		return true, true
	}
	if strings.HasPrefix(from, "[") && strings.HasPrefix(to, "[") {
		fromEnd, toEnd := strings.IndexByte(from, ']'), strings.IndexByte(to, ']')
		if (fromEnd > 1) != (toEnd > 1) {
			return false, true
		}
		if fromEnd > 1 && toEnd > 1 && from[fromEnd+1:] != "" && to[toEnd+1:] != "" {
			if from[:fromEnd] != to[:toEnd] {
				return false, true
			}
			fromElement, toElement := from[fromEnd+1:], to[toEnd+1:]
			if fromElement == toElement {
				return true, true
			}
			left, leftKnown := e.canonicalArrayElement(fromElement, map[string]bool{})
			right, rightKnown := e.canonicalArrayElement(toElement, map[string]bool{})
			if leftKnown && rightKnown {
				return left == right, true
			}
		}
	}
	if to == "any" || to == "interface{}" || to == "interface {}" {
		return true, true
	}
	if from == "nil" {
		return strings.HasPrefix(to, "*") || strings.HasPrefix(to, "[]") || strings.HasPrefix(to, "map[") || strings.HasPrefix(to, "chan ") || strings.HasPrefix(to, "func(") || to == "error", true
	}
	if strings.HasPrefix(from, "untyped ") {
		switch strings.TrimPrefix(from, "untyped ") {
		case "string":
			if builtin(to) {
				return to == "string", true
			}
		case "bool":
			if builtin(to) {
				return to == "bool", true
			}
		case "int", "rune":
			if numeric(to) {
				return true, true
			}
		case "float":
			if to == "float32" || to == "float64" || to == "complex64" || to == "complex128" {
				return true, true
			}
		case "complex":
			return to == "complex64" || to == "complex128", true
		}
	}
	if (from == "byte" && to == "uint8") || (from == "uint8" && to == "byte") || (from == "rune" && to == "int32") || (from == "int32" && to == "rune") {
		return true, true
	}
	key := from + "\x00" + to
	if seen[key] {
		return false, false
	}
	seen[key] = true
	for _, name := range []string{from, to} {
		base := receiverID(name)
		if i := strings.LastIndex(base, "."); i > 0 {
			_ = e.definitions(base[:i], base[i+1:])
		}
	}
	if typ, ok := e.types[to]; ok {
		if typ.Alias {
			return e.assignable(from, typ.Underlying, seen)
		}
		if typ.Underlying == "interface" || len(typ.Methods) > 0 && strings.HasPrefix(typ.Underlying, "interface") {
			if len(typ.Embedded) > 0 {
				return false, false
			}
			for name, signature := range typ.Methods {
				actual, found, known := e.interfaceMethod(from, name)
				if !known {
					return false, false
				}
				if !found {
					return false, true
				}
				if len(actual.Params) != len(signature.Params) || len(actual.Results) != len(signature.Results) || actual.Variadic != signature.Variadic {
					return false, true
				}
				for i, p := range actual.Params {
					if p.Type.Name == "" || signature.Params[i].Type.Name == "" {
						return false, false
					}
					if p.Type.Name != signature.Params[i].Type.Name {
						return false, true
					}
				}
				for i, p := range actual.Results {
					if p.Type.Name == "" || signature.Results[i].Type.Name == "" {
						return false, false
					}
					if p.Type.Name != signature.Results[i].Type.Name {
						return false, true
					}
				}
			}
			return true, true
		}
		if strings.HasPrefix(from, "untyped ") {
			return e.assignable(from, typ.Underlying, seen)
		}
	}
	if typ, ok := e.types[from]; ok && typ.Alias {
		return e.assignable(typ.Underlying, to, seen)
	}
	// Only reject fully known simple types. Opaque type parameters and complex
	// expressions need further type information rather than a guessed failure.
	if builtin(from) && builtin(to) {
		return false, true
	}
	if strings.Contains(from, ".") && strings.Contains(to, ".") && !strings.ContainsAny(from+to, "[](){}") {
		_, a := e.types[receiverID(from)]
		_, b := e.types[receiverID(to)]
		return false, a && b
	}
	if strings.HasPrefix(from, "*") != strings.HasPrefix(to, "*") && strings.TrimPrefix(from, "*") == strings.TrimPrefix(to, "*") {
		return false, true
	}
	return false, false
}

// canonicalArrayElement expands aliases while preserving the identity of
// defined types. Array elements must be identical, not merely assignable.
func (e *engine) canonicalArrayElement(name string, seen map[string]bool) (string, bool) {
	if name == "" || seen[name] {
		return "", false
	}
	seen[name] = true
	if strings.HasPrefix(name, "*") || strings.HasPrefix(name, "[]") {
		prefix := "*"
		if strings.HasPrefix(name, "[]") {
			prefix = "[]"
		}
		base, known := e.canonicalArrayElement(strings.TrimPrefix(name, prefix), seen)
		return prefix + base, known
	}
	if strings.HasPrefix(name, "[") {
		end := strings.IndexByte(name, ']')
		if end > 1 && end+1 < len(name) {
			base, known := e.canonicalArrayElement(name[end+1:], seen)
			return name[:end+1] + base, known
		}
		return "", false
	}
	switch name {
	case "byte":
		return "uint8", true
	case "rune":
		return "int32", true
	case "any", "interface{}", "interface {}":
		return "interface{}", true
	}
	if builtin(name) {
		return name, true
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		_ = e.definitions(name[:i], name[i+1:])
	}
	typ, ok := e.types[name]
	if !ok {
		return "", false
	}
	if typ.Alias {
		return e.canonicalArrayElement(typ.Underlying, seen)
	}
	return name, true
}

// interfaceMethod finds a method in the Go method set of a named type. At each
// embedding depth, fields and methods with the same name shadow deeper methods;
// multiple candidates at that depth are ambiguous.
func (e *engine) interfaceMethod(from, name string) (Signature, bool, bool) {
	type candidate struct {
		name    string
		pointer bool
		seen    map[string]bool
	}
	queue := []candidate{{name: receiverID(from), pointer: strings.HasPrefix(from, "*"), seen: map[string]bool{}}}
	for len(queue) > 0 {
		var next []candidate
		var matches []Signature
		declarations := 0
		unknown := false
		for _, item := range queue {
			if item.name == "" || item.seen[item.name] {
				continue
			}
			seen := make(map[string]bool, len(item.seen)+1)
			for key := range item.seen {
				seen[key] = true
			}
			seen[item.name] = true
			i := strings.LastIndex(item.name, ".")
			if i > 0 {
				_ = e.definitions(item.name[:i], item.name[i+1:])
				_ = e.definitions(item.name[:i], name)
			}
			typ, ok := e.types[item.name]
			if !ok {
				if builtin(item.name) || item.name == "any" || item.name == "interface{}" || item.name == "interface {}" {
					continue
				}
				if item.name == "error" {
					if name == "Error" {
						declarations++
						matches = append(matches, Signature{Results: []Parameter{{Type: TypeRef{Name: "string"}}}})
					}
					continue
				}
				unknown = true
				continue
			}
			if typ.Alias && typ.Underlying != "" {
				underlying := typ.Underlying
				next = append(next, candidate{name: receiverID(underlying), pointer: item.pointer || strings.HasPrefix(underlying, "*"), seen: seen})
				continue
			}
			if _, field := typ.Fields[name]; field {
				declarations++
			}
			if f, exists := e.functions[item.name+"#"+name]; exists {
				declarations++
				if !strings.HasPrefix(f.Receiver, "*") || item.pointer {
					matches = append(matches, Signature{Params: f.Params, Results: f.Results, Variadic: f.Variadic})
				}
			} else if signature, exists := typ.Methods[name]; exists {
				declarations++
				matches = append(matches, signature)
			}
			for _, embedded := range typ.Embedded {
				next = append(next, candidate{name: receiverID(embedded.Name), pointer: item.pointer || strings.HasPrefix(embedded.Name, "*"), seen: seen})
			}
		}
		if unknown {
			return Signature{}, false, false
		}
		if declarations > 0 {
			if declarations == 1 && len(matches) == 1 {
				return matches[0], true, true
			}
			return Signature{}, false, true
		}
		queue = next
	}
	return Signature{}, false, true
}

func numeric(s string) bool {
	switch s {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune", "float32", "float64", "complex64", "complex128":
		return true
	}
	return false
}
func builtin(s string) bool { return numeric(s) || s == "bool" || s == "string" }

// argumentAssignable checks representability as well as the type of constants.
func (e *engine) argumentAssignable(arg TypeRef, to string) (bool, bool) {
	arg = e.resolveType(arg, map[string]bool{})
	if !strings.HasPrefix(arg.Name, "untyped ") {
		return e.assignable(arg.Name, to, map[string]bool{})
	}
	base := to
	seen := map[string]bool{}
	for !builtin(base) && base != "any" && base != "interface{}" {
		if seen[base] {
			return false, false
		}
		seen[base] = true
		if i := strings.LastIndex(base, "."); i > 0 {
			_ = e.definitions(base[:i], base[i+1:])
		}
		t, ok := e.types[base]
		if !ok || t.Underlying == "" {
			return e.assignable(arg.Name, to, map[string]bool{})
		}
		base = t.Underlying
	}
	match, known := e.assignable(arg.Name, base, map[string]bool{})
	if numeric(base) && (arg.Name == "untyped int" || arg.Name == "untyped rune" || arg.Name == "untyped float" || arg.Name == "untyped complex") {
		match, known = true, true
	}
	if !match {
		if builtin(base) {
			return false, true
		}
		return match, known
	}
	if !numeric(base) {
		return match, known
	}
	if arg.Value == "" {
		return false, false
	}
	kind := token.INT
	switch strings.TrimPrefix(arg.Name, "untyped ") {
	case "rune":
		kind = token.CHAR
	case "float":
		kind = token.FLOAT
	case "complex":
		kind = token.IMAG
	}
	literal := arg.Value
	negative := strings.HasPrefix(literal, "-")
	literal = strings.TrimPrefix(literal, "-")
	value := constant.MakeFromLiteral(literal, kind, 0)
	if value.Kind() == constant.Unknown {
		return false, false
	}
	if negative {
		value = constant.UnaryOp(token.SUB, value, 0)
	}
	switch base {
	case "float32", "float64":
		value = constant.ToFloat(value)
		if value.Kind() == constant.Unknown {
			return false, true
		}
		if base == "float32" {
			v, _ := constant.Float32Val(value)
			return v <= 3.4028234663852886e38 && v >= -3.4028234663852886e38, true
		}
		v, _ := constant.Float64Val(value)
		return v <= 1.7976931348623157e308 && v >= -1.7976931348623157e308, true
	case "complex64", "complex128":
		return true, true
	}
	value = constant.ToInt(value)
	if value.Kind() == constant.Unknown {
		return false, true
	}
	bits := 0
	unsigned := strings.HasPrefix(base, "uint") || base == "byte" || base == "uintptr"
	switch base {
	case "byte":
		bits = 8
	case "rune":
		bits = 32
	case "int", "uint", "uintptr":
		switch e.build.GOARCH {
		case "386", "arm", "mips", "mipsle":
			bits = 32
		case "":
			if runtime.GOARCH == "386" || runtime.GOARCH == "arm" {
				bits = 32
			} else {
				bits = 64
			}
		default:
			bits = 64
		}
	default:
		bits, _ = strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(base, "u"), "int"))
	}
	if bits == 0 {
		return false, false
	}
	limit := constant.Shift(constant.MakeInt64(1), token.SHL, uint(bits))
	minimum := constant.MakeInt64(0)
	if !unsigned {
		limit = constant.Shift(constant.MakeInt64(1), token.SHL, uint(bits-1))
		minimum = constant.UnaryOp(token.SUB, limit, 0)
	}
	return constant.Compare(value, token.GEQ, minimum) && constant.Compare(value, token.LSS, limit), true
}
