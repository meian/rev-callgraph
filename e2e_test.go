package main_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
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
}
