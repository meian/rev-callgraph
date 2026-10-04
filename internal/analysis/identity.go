package analysis

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// packageScope は構文変換と型比較に共通の内部名前空間を与える。
// 区切りと符号化した identity は Go の import path に現れず、公開 ID には含めない。
func packageScope(path, identity string) string {
	if identity == "" {
		return path
	}
	return path + "\x00" + hex.EncodeToString([]byte(identity)) + "\x00"
}

// splitPackageScope は内部名前空間から公開 path と package identity を取り出す。
func splitPackageScope(path string) (string, string) {
	start := strings.IndexByte(path, 0)
	if start < 0 || !strings.HasSuffix(path, "\x00") {
		return path, ""
	}
	identity, err := hex.DecodeString(path[start+1 : len(path)-1])
	if err != nil {
		return path, ""
	}
	return path[:start], string(identity)
}

// publicSymbol は内部シンボルを従来の表示用 ID に戻す。
func publicSymbol(id string) string {
	target, err := ParseTarget(id)
	if err != nil {
		return id
	}
	target.Package, _ = splitPackageScope(target.Package)
	return target.ID()
}

// importPackage は path と既存の module 対応から通常 package を選ぶ。
// 宣言名や近隣ディレクトリを検索せず、同順位の候補が複数あれば確定しない。
func importPackage(w *Workspace, module int, path string) (Package, bool) {
	var selected Package
	best, count := 0, 0
	for _, pkg := range w.Packages {
		if pkg.Path != path || pkg.ExternalTest {
			continue
		}
		rank := 1
		if pkg.Module == module {
			rank = 3
		} else if module >= 0 && module < len(w.Modules) && pkg.Module >= 0 && pkg.Module < len(w.Modules) {
			consumer := w.Modules[module]
			for required := range consumer.Requires {
				if requireTargetsModule(consumer, required, w.Modules[pkg.Module]) {
					rank = 2
				}
			}
			for required := range consumer.Replaces {
				if requireTargetsModule(consumer, required, w.Modules[pkg.Module]) {
					rank = 2
				}
			}
		}
		if rank > best {
			selected, best, count = pkg, rank, 1
		} else if rank == best {
			count++
		}
	}
	return selected, count == 1
}

// scopedImports は同じ module 内で共有する内部 import 対応を保持する。
type scopedImports struct {
	paths, names map[string]string
}

// scopedSource は engine 内の構文変換だけに使うコピーを作る。
// ローカル名と import 名を変換時に分離し、複合型・別名・関数値にも同じ identity を伝播する。
// 公開 AnalyzeSource の ID と、発見時の Source は変更しない。
func (e *engine) scopedSource(source Source) Source {
	source.Package = packageScope(source.Package, source.PackageID)
	if e.workspace == nil {
		return source
	}
	if imports, ok := e.importScopes[source.Module]; ok {
		source.ImportPaths, source.PackageNames = imports.paths, imports.names
		return source
	}
	paths := make(map[string]string)
	names := make(map[string]string)
	for path, name := range source.PackageNames {
		names[path] = name
	}
	for path, canonical := range source.ImportPaths {
		paths[path] = canonical
	}
	for _, pkg := range e.workspace.Packages {
		if selected, ok := importPackage(e.workspace, source.Module, pkg.Path); ok {
			scope := packageScope(selected.Path, selected.ID)
			paths[pkg.Path] = scope
			names[scope] = selected.Name
		}
	}
	for path, canonical := range source.ImportPaths {
		if scope := paths[canonical]; scope != "" {
			paths[path] = scope
		}
	}
	if e.importScopes == nil {
		e.importScopes = make(map[int]scopedImports)
	}
	e.importScopes[source.Module] = scopedImports{
		paths: paths,
		names: names,
	}
	source.ImportPaths, source.PackageNames = paths, names
	return source
}

// targetSymbol は実在する定義から起点を選び、同じ公開 ID の複数定義を拒否する。
// 定義がない起点は削除 API の検索用として公開 ID のまま保持する。
func (e *engine) targetSymbol(target Target) (string, error) {
	var candidates []string
	for _, pkg := range e.workspace.Packages {
		if pkg.Path != target.Package {
			continue
		}
		scoped := target
		scoped.Package = packageScope(pkg.Path, pkg.ID)
		if err := e.definitions(scoped.Package, target.Name); err != nil {
			return "", err
		}
		_, exists := e.functions[scoped.ID()]
		if !exists && scoped.Receiver != "" {
			var err error
			_, exists, err = e.method(scoped.Package+"."+scoped.Receiver, scoped.Name, map[string]bool{})
			if err != nil {
				return "", err
			}
		}
		if exists {
			candidates = append(candidates, scoped.ID())
		}
	}
	if len(candidates) > 1 {
		var locations []string
		for _, id := range candidates {
			locations = append(locations, e.symbolLabel(id))
		}
		sort.Strings(locations)
		return "", fmt.Errorf("ambiguous target %s: %s", target.ID(), strings.Join(locations, ", "))
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return target.ID(), nil
}

// symbolLabel は公開 ID に所在地と package 種別を付けた表示名を返す。
func (e *engine) symbolLabel(id string) string {
	target, err := ParseTarget(id)
	if err != nil {
		return id
	}
	_, identity := splitPackageScope(target.Package)
	for _, pkg := range e.workspace.Packages {
		if pkg.ID == identity {
			kind := "package"
			if pkg.ExternalTest {
				kind = "external-test"
			}
			return publicSymbol(id) + " [" + filepath.ToSlash(pkg.Dir) + "; " + kind + "]"
		}
	}
	return publicSymbol(id)
}

// displayResult は探索後にだけ公開名へ戻す。
// 実際の結果に現れた同名シンボルは所在地を付記し、出力側の集約でも区別する。
func (e *engine) displayResult(result *Result) {
	identities := map[string]map[string]bool{}
	register := func(id string) {
		public := publicSymbol(id)
		if identities[public] == nil {
			identities[public] = map[string]bool{}
		}
		identities[public][id] = true
	}
	register(result.Root.Name)
	for _, edge := range result.Edges {
		register(edge.Caller)
		register(edge.Callee)
	}
	label := func(id string) string {
		public := publicSymbol(id)
		if len(identities[public]) > 1 {
			return e.symbolLabel(id)
		}
		return public
	}
	var replacements []string
	for _, pkg := range e.workspace.Packages {
		replacements = append(replacements, packageScope(pkg.Path, pkg.ID), pkg.Path)
	}
	diagnostics := strings.NewReplacer(replacements...)
	convertEdge := func(edge *Edge) {
		edge.Caller, edge.Callee = label(edge.Caller), label(edge.Callee)
		for i := range edge.Compatibility.Issues {
			edge.Compatibility.Issues[i].Message = diagnostics.Replace(edge.Compatibility.Issues[i].Message)
		}
	}
	var visit func(*Node)
	visit = func(node *Node) {
		node.Name = label(node.Name)
		if node.Edge != nil {
			convertEdge(node.Edge)
		}
		for _, child := range node.Callers {
			visit(child)
		}
	}
	visit(result.Root)
	for i := range result.Edges {
		convertEdge(&result.Edges[i])
	}
}

// matchesPackage は内部参照では identity、公開の未解決参照では通常 path を比較する。
func matchesPackage(pkg Package, reference string) bool {
	path, identity := splitPackageScope(reference)
	if identity != "" {
		return pkg.ID == identity
	}
	return !pkg.ExternalTest && pkg.Path == path
}
