package main_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E(t *testing.T) {
	// rev-callgraph CLIをgo runで実行
	cmd := exec.Command("go", "run", ".", "github.com/meian/rev-callgraph/testdata/foo.Target", "--dir", "testdata", "--format", "json", "--json-style", "edges")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("CLI実行失敗: %v, 出力: %s", err, out.String())
	}
	// 標準出力からJSON部分を抽出
	output := out.String()
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("JSON出力が見つかりません: %s", output)
	}
	jsonText := output[idx:]
	// JSON構造をパース
	var result struct {
		Root  string              `json:"root"`
		Edges []map[string]string `json:"edges"`
	}
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("JSONパース失敗: %v, raw: %s", err, jsonText)
	}
	// rootはターゲットと一致
	if result.Root != "github.com/meian/rev-callgraph/testdata/foo.Target" {
		t.Errorf("Unexpected root: %s", result.Root)
	}
	// 呼び出し元にexample.com/bar.Callerが含まれること
	found := false
	for _, e := range result.Edges {
		if e["caller"] == "github.com/meian/rev-callgraph/testdata/bar.Caller" && e["callee"] == "github.com/meian/rev-callgraph/testdata/foo.Target" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected edge not found: bar.Caller -> foo.Target, got edges: %#v", result.Edges)
	}
	methodExpression := false
	for _, e := range result.Edges {
		if e["caller"] == "github.com/meian/rev-callgraph/testdata/bar.ExpressionCaller" && e["callee"] == "github.com/meian/rev-callgraph/testdata/foo.SomeStruct#Method" {
			methodExpression = true
			break
		}
	}
	if !methodExpression {
		t.Errorf("Expected method expression edge not found: bar.ExpressionCaller -> foo.SomeStruct#Method, got edges: %#v", result.Edges)
	}
}

func TestCLIFormatsAndProgress(t *testing.T) {
	target := "github.com/meian/rev-callgraph/testdata/foo.Target"
	cmd := exec.Command("go", "run", ".", target, "--dir", "testdata", "--format", "dot", "--max-depth", "1", "--progress")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("dot CLI: %v, stderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "digraph rev_callgraph {") || !strings.Contains(stdout.String(), "->") {
		t.Fatalf("DOT graph missing from stdout: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "analyzing ") || !strings.Contains(stderr.String(), "analyzing ") {
		t.Fatalf("progress must be on stderr: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLISymbolSetAndBuildContext(t *testing.T) {
	dir := t.TempDir()
	writeFixture := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture("go.mod", "module example.com/cli-fixture\n\ngo 1.24\n")
	writeFixture("target.go", "package fixture\nfunc Target() {}\nfunc RuntimeCaller() { Target() }\n")
	writeFixture("target_test.go", "package fixture\nimport \"testing\"\nfunc TestTarget(t *testing.T) { Target() }\n")
	writeFixture("caller_linux.go", "package fixture\nfunc LinuxCaller() { Target() }\n")
	writeFixture("caller_darwin.go", "package fixture\nfunc DarwinCaller() { Target() }\n")
	target := "example.com/cli-fixture.Target"
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("go", append([]string{"run", ".", target, "--dir", dir, "--format", "json", "--json-style", "edges"}, args...)...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("CLI %v: %v, stderr: %s", args, err, stderr.String())
		}
		return stdout.String()
	}
	runtime := run("--symbol-set", "runtime", "--goos", "linux", "--goarch", "amd64")
	if !strings.Contains(runtime, "LinuxCaller") || strings.Contains(runtime, "DarwinCaller") || strings.Contains(runtime, "TestTarget") {
		t.Errorf("linux runtime callers: %s", runtime)
	}
	test := run("--symbol-set", "test", "--goos", "linux", "--goarch", "amd64")
	if !strings.Contains(test, "LinuxCaller") || !strings.Contains(test, "TestTarget") || strings.Contains(test, "DarwinCaller") {
		t.Errorf("linux test callers: %s", test)
	}
}

func TestCLIMixedCompatibilityKeepsValidRoute(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/mixed\ngo 1.26\n",
		"p.go":   "package mixed\nfunc Target(x int){}\nfunc B(){Target(1);Target();Target(2)}\nfunc A(){B()}",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("go", "run", ".", "example.com/mixed.Target", "--dir", dir, "--format", "json", "--json-style", "edges")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI: %v: %s", err, out)
	}
	var result struct {
		Edges []struct {
			Caller, Callee string
			Compatibility  *struct{ Status string }
		}
	}
	if err = json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	valid, invalid, above := 0, 0, 0
	for _, edge := range result.Edges {
		if edge.Caller == "example.com/mixed.B" && edge.Callee == "example.com/mixed.Target" {
			if edge.Compatibility == nil {
				valid++
			} else if edge.Compatibility.Status == "incompatible" {
				invalid++
			}
		}
		if edge.Caller == "example.com/mixed.A" && edge.Callee == "example.com/mixed.B" {
			above++
		}
	}
	if valid != 1 || invalid != 1 || above != 1 {
		t.Fatalf("normal route or issue lost: %s", out)
	}
}

func TestCLICollidingExternalTestPackages(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":               "module example.com/collision\ngo 1.24\n",
		"foo/regular.go":       "package foo\nfunc Target() {}\n",
		"foo/external_test.go": "package foo_test\nimport \"example.com/collision/foo\"\nfunc Caller() { foo.Target() }\n",
		"foo_test/regular.go":  "package foo_test\nimport \"example.com/collision/foo\"\nfunc Caller() { foo.Target() }\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, format := range []string{"tree", "json", "dot"} {
		t.Run(format, func(t *testing.T) {
			cmd := exec.Command("go", "run", ".", "example.com/collision/foo.Target", "--dir", dir, "--symbol-set", "test", "--format", format, "--json-style", "edges")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("CLI %s: %v: %s", format, err, stderr.String())
			}
			text := stdout.String()
			if !strings.Contains(text, "; external-test]") || !strings.Contains(text, "; package]") || strings.Contains(text, `\u0000`) || strings.ContainsRune(text, 0) {
				t.Fatalf("package identities missing or leaked: %s", text)
			}
			if format == "json" {
				var document struct {
					Nodes []string `json:"nodes"`
					Edges []struct {
						Caller string `json:"caller"`
					} `json:"edges"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
					t.Fatal(err)
				}
				if len(document.Nodes) != 3 || len(document.Edges) != 2 || document.Edges[0].Caller == document.Edges[1].Caller {
					t.Fatalf("colliding JSON nodes merged: %+v", document)
				}
			}
		})
	}
	cmd := exec.Command("go", "run", ".", "example.com/collision/foo_test.Caller", "--dir", dir, "--symbol-set", "test")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil || !strings.Contains(stderr.String(), "ambiguous target") || stdout.Len() != 0 {
		t.Fatalf("ambiguous target: stdout=%q stderr=%q err=%v", stdout.String(), stderr.String(), err)
	}
}
