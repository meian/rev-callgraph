package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/meian/rev-callgraph/internal/analysis"
)

func TestJSONMetadataAndOrder(t *testing.T) {
	resolved := analysis.Edge{Caller: "z", Callee: "target", Resolution: analysis.Resolution{Status: analysis.Resolved}, Compatibility: analysis.Compatibility{Status: analysis.Compatible}}
	broken := analysis.Edge{Caller: "a", Callee: "target", Resolution: analysis.Resolution{Status: analysis.Resolved}, Compatibility: analysis.Compatibility{Status: analysis.Incompatible, Issues: []analysis.Issue{{Kind: "argument-count", Message: "want 2"}}}}
	result := &analysis.Result{Root: &analysis.Node{Name: "target", Callers: []*analysis.Node{{Name: "z", Edge: &resolved}, {Name: "a", Edge: &broken}}}, Edges: []analysis.Edge{resolved, broken}}
	var out bytes.Buffer
	if err := Write(&out, result, "json", "edges"); err != nil {
		t.Fatal(err)
	}
	var document struct {
		Nodes []string                     `json:"nodes"`
		Edges []map[string]json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if strings.Join(document.Nodes, ",") != "a,target,z" {
		t.Errorf("nodes not sorted: %v", document.Nodes)
	}
	if len(document.Edges) != 2 || string(document.Edges[0]["caller"]) != `"a"` {
		t.Fatalf("edges not sorted: %s", out.String())
	}
	if _, ok := document.Edges[0]["compatibility"]; !ok {
		t.Errorf("incompatible edge missing metadata: %s", out.String())
	}
	if _, ok := document.Edges[1]["compatibility"]; ok {
		t.Errorf("compatible edge changed legacy shape: %s", out.String())
	}
	out.Reset()
	if err := Write(&out, result, "json", "nested"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"issues"`) || !strings.Contains(out.String(), `"argument-count"`) {
		t.Errorf("nested metadata missing: %s", out.String())
	}
}

func TestTreeAndDOTShowStatus(t *testing.T) {
	edge := analysis.Edge{Caller: "caller", Callee: "target", Resolution: analysis.Resolution{Status: analysis.External}, Compatibility: analysis.Compatibility{Status: analysis.CompatibilityUnknown}}
	result := &analysis.Result{Root: &analysis.Node{Name: "target", Callers: []*analysis.Node{{Name: "caller", Main: true, Cycle: true, Edge: &edge}}}, Edges: []analysis.Edge{edge}}
	var out bytes.Buffer
	if err := Write(&out, result, "tree", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "caller [main] (cycled) [resolution=external, compatibility=unknown]") {
		t.Errorf("tree status missing: %s", out.String())
	}
	out.Reset()
	if err := Write(&out, result, "dot", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"caller" -> "target" [label="external / unknown"]`) {
		t.Errorf("DOT status missing: %s", out.String())
	}
}
