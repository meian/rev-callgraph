package analysis

import (
	"context"
	"fmt"
	"go/build"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// Discover finds Go modules and files eligible for the requested build and symbol set.
func Discover(ctx context.Context, options Options) (*Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := options.Dir
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory: %s", root)
	}
	if options.SymbolSet != "" && options.SymbolSet != Runtime && options.SymbolSet != Test {
		return nil, fmt.Errorf("invalid symbol set %q", options.SymbolSet)
	}

	w := &Workspace{}
	modDirs := make(map[string]int)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := modfile.Parse(path, data, nil)
		if err != nil {
			return err
		}
		if parsed.Module == nil {
			return fmt.Errorf("%s: missing module directive", path)
		}
		m := Module{Path: parsed.Module.Mod.Path, Dir: filepath.Dir(path), Requires: make(map[string]string), Replaces: make(map[string][]Replacement)}
		for _, req := range parsed.Require {
			m.Requires[req.Mod.Path] = req.Mod.Version
		}
		for _, replace := range parsed.Replace {
			m.Replaces[replace.Old.Path] = append(m.Replaces[replace.Old.Path], Replacement{
				OldVersion: replace.Old.Version,
				NewPath:    replace.New.Path,
				NewVersion: replace.New.Version,
			})
		}
		modDirs[m.Dir] = len(w.Modules)
		w.Modules = append(w.Modules, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(w.Modules) == 0 {
		return nil, fmt.Errorf("no go.mod found under %s", root)
	}
	assignModuleSeries(w)

	buildContext := build.Default
	if options.Build.GOOS != "" {
		buildContext.GOOS = options.Build.GOOS
	}
	if options.Build.GOARCH != "" {
		buildContext.GOARCH = options.Build.GOARCH
	}
	if options.Build.CgoSet {
		buildContext.CgoEnabled = options.Build.Cgo
	} else if buildContext.GOOS != build.Default.GOOS || buildContext.GOARCH != build.Default.GOARCH {
		buildContext.CgoEnabled = false
	}
	buildContext.BuildTags = append([]string(nil), options.Build.Tags...)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		test := strings.HasSuffix(entry.Name(), "_test.go")
		if test && options.SymbolSet != Test {
			return nil
		}
		module := -1
		for dir, index := range modDirs {
			if pathWithin(dir, path) && (module < 0 || len(dir) > len(w.Modules[module].Dir)) {
				module = index
			}
		}
		if module < 0 {
			return nil
		}
		matches, err := buildContext.MatchFile(filepath.Dir(path), entry.Name())
		if err != nil {
			return err
		}
		if !matches {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		if !buildContext.CgoEnabled {
			for _, imported := range file.Imports {
				if imported.Path.Value == `"C"` {
					return nil
				}
			}
		}
		rel, err := filepath.Rel(w.Modules[module].Dir, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := w.Modules[module].Path
		if rel != "." {
			pkg += "/" + filepath.ToSlash(rel)
		}
		if test && strings.HasSuffix(file.Name.Name, "_test") {
			pkg += "_test"
		}
		w.Sources = append(w.Sources, Source{Path: path, Package: pkg, PackageName: file.Name.Name, Module: module, Test: test})
		return nil
	})
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, source := range w.Sources {
		names[source.Package] = source.PackageName
	}
	for i := range w.Sources {
		w.Sources[i].PackageNames = names
		consumer := w.Modules[w.Sources[i].Module]
		aliases := map[string]string{}
		for required := range consumer.Replaces {
			for _, module := range w.Modules {
				if requireTargetsModule(consumer, required, module) {
					for _, source := range w.Sources {
						if source.Package == module.Path || strings.HasPrefix(source.Package, module.Path+"/") {
							aliases[required+strings.TrimPrefix(source.Package, module.Path)] = source.Package
						}
					}
				}
			}
		}
		w.Sources[i].ImportPaths = aliases
	}
	return w, nil
}

func pathWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func assignModuleSeries(w *Workspace) {
	for i := range w.Modules {
		module := &w.Modules[i]
		if major := modulePathMajor(module.Path); major != "" {
			module.Series = major
			continue
		}
		majors := make(map[string]bool)
		for _, consumer := range w.Modules {
			for requiredPath, version := range consumer.Requires {
				if !requireTargetsModule(consumer, requiredPath, *module) {
					continue
				}
				if replacement, ok := selectedReplacement(consumer, requiredPath); ok && replacement.NewVersion != "" {
					version = replacement.NewVersion
				}
				if major := semver.Major(version); major != "" {
					majors[major] = true
				}
			}
		}
		if len(majors) == 1 {
			for major := range majors {
				module.Series = major
			}
		}
	}
}

func requireTargetsModule(consumer Module, requiredPath string, module Module) bool {
	replacement, ok := selectedReplacement(consumer, requiredPath)
	if !ok {
		return requiredPath == module.Path
	}
	if replacement.NewPath == module.Path {
		return true
	}
	if !modfile.IsDirectoryPath(replacement.NewPath) {
		return false
	}
	path := replacement.NewPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(consumer.Dir, path)
	}
	return filepath.Clean(path) == module.Dir
}

// A replacement for the selected require version takes precedence over a
// replacement covering every version. Without a require, only the latter applies.
func selectedReplacement(consumer Module, requiredPath string) (Replacement, bool) {
	version := consumer.Requires[requiredPath]
	var allVersions Replacement
	hasAllVersions := false
	for _, replacement := range consumer.Replaces[requiredPath] {
		if version != "" && replacement.OldVersion == version {
			return replacement, true
		}
		if replacement.OldVersion == "" {
			allVersions = replacement
			hasAllVersions = true
		}
	}
	return allVersions, hasAllVersions
}

func modulePathMajor(path string) string {
	last := path[strings.LastIndex(path, "/")+1:]
	if strings.HasPrefix(path, "gopkg.in/") {
		if i := strings.LastIndex(last, ".v"); i >= 0 {
			if major := versionSuffix(last[i+1:], 0); major != "" {
				return major
			}
		}
	}
	return versionSuffix(last, 2)
}

func versionSuffix(last string, minimum int) string {
	if len(last) < 2 || last[0] != 'v' {
		return ""
	}
	n, err := strconv.Atoi(last[1:])
	if err != nil || n < minimum || strconv.Itoa(n) != last[1:] {
		return ""
	}
	return last
}

type indexedLocator struct {
	definitions map[string]map[string][]Source
	callers     map[string][]Source
}

// NewLocator builds a token index without parsing whole source files into ASTs.
func NewLocator(w *Workspace) (Locator, error) {
	if w == nil {
		return nil, fmt.Errorf("nil workspace")
	}
	index := &indexedLocator{definitions: make(map[string]map[string][]Source), callers: make(map[string][]Source)}
	for _, source := range w.Sources {
		data, err := os.ReadFile(source.Path)
		if err != nil {
			return nil, err
		}
		var scan scanner.Scanner
		files := token.NewFileSet()
		file := files.AddFile(source.Path, files.Base(), len(data))
		scan.Init(file, data, nil, scanner.ScanComments)
		var tokens []token.Token
		var literals []string
		for {
			_, tok, literal := scan.Scan()
			if tok == token.EOF {
				break
			}
			if tok == token.COMMENT {
				continue
			}
			tokens = append(tokens, tok)
			literals = append(literals, literal)
		}
		definitions := make(map[string]bool)
		calls := make(map[string]bool)
		group := token.ILLEGAL
		groupDepth := 0
		groupStart := false
		for i, tok := range tokens {
			if group != token.ILLEGAL {
				if tok == token.LPAREN {
					groupDepth++
				} else if tok == token.RPAREN {
					groupDepth--
					if groupDepth == 0 {
						group = token.ILLEGAL
					}
				} else if groupDepth == 1 && tok == token.SEMICOLON {
					groupStart = true
				} else if groupDepth == 1 && groupStart && tok == token.IDENT {
					definitions[literals[i]] = true
					groupStart = false
				}
			}
			if tok == token.FUNC && i+1 < len(tokens) {
				j := i + 1
				if tokens[j] == token.LPAREN {
					depth := 1
					for j++; j < len(tokens) && depth > 0; j++ {
						switch tokens[j] {
						case token.LPAREN:
							depth++
						case token.RPAREN:
							depth--
						}
					}
				}
				if j < len(tokens) && tokens[j] == token.IDENT {
					definitions[literals[j]] = true
				}
			}
			if (tok == token.TYPE || tok == token.VAR || tok == token.CONST) && i+1 < len(tokens) {
				if tokens[i+1] == token.IDENT {
					definitions[literals[i+1]] = true
				} else if tokens[i+1] == token.LPAREN {
					group = tok
					groupDepth = 0
					groupStart = true
				}
			}
			if tok == token.IDENT {
				calls[literals[i]] = true
			}
		}
		if index.definitions[source.Package] == nil {
			index.definitions[source.Package] = make(map[string][]Source)
		}
		for name := range definitions {
			index.definitions[source.Package][name] = append(index.definitions[source.Package][name], source)
		}
		for name := range calls {
			index.callers[name] = append(index.callers[name], source)
		}
	}
	return index, nil
}

func (l *indexedLocator) Definitions(packagePath, name string) []Source {
	return append([]Source(nil), l.definitions[packagePath][name]...)
}

func (l *indexedLocator) Callers(_ string, name string) []Source {
	return append([]Source(nil), l.callers[name]...)
}

var _ Locator = (*indexedLocator)(nil)
