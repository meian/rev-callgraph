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
