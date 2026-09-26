package analysis

import (
	"fmt"
	"testing"
)

func TestReviewMixedCallSitesPreserveCompatiblePath(t *testing.T) {
	result := runFixture(t, map[string]string{
		"go.mod": "module example.com/p\ngo 1.26\n",
		"p.go":   "package p\nfunc Target(x int){}\nfunc B(){Target(1);Target();Target(2)}\nfunc A(){B()}",
	}, "example.com/p.Target", Options{})
	if len(result.Root.Callers) != 2 {
		t.Fatalf("want two distinct outcomes, got %+v", result.Root.Callers)
	}
	statuses := map[CompatibilityStatus]bool{}
	for _, node := range result.Root.Callers {
		statuses[node.Edge.Compatibility.Status] = true
		switch node.Edge.Compatibility.Status {
		case Compatible:
			if len(node.Callers) != 1 || node.Callers[0].Name != "example.com/p.A" {
				t.Fatalf("valid path lost: %+v", node)
			}
		case Incompatible:
			if len(node.Callers) != 0 {
				t.Fatal("incompatible path traversed")
			}
		default:
			t.Fatalf("unexpected status %+v", node.Edge)
		}
	}
	if !statuses[Compatible] || !statuses[Incompatible] {
		t.Fatal(statuses)
	}
	if len(result.Edges) != 3 {
		t.Fatalf("duplicate equivalent calls or lost path: %+v", result.Edges)
	}
}

func TestReviewLazySkipsUnrelatedOutgoingCalls(t *testing.T) {
	files := map[string]string{"go.mod": "module example.com/p\ngo 1.26\n"}
	body := "package p\nfunc Target(){}\nfunc Caller(){Target();"
	for i := 0; i < 300; i++ {
		body += fmt.Sprintf("Other%d();", i)
		files[fmt.Sprintf("other%03d.go", i)] = fmt.Sprintf("package p\nfunc Other%d(){}\n", i)
	}
	files["target.go"] = body + "}\n"
	result := runFixture(t, files, "example.com/p.Target", Options{})
	if len(result.Edges) != 1 {
		t.Fatalf("missing caller: %+v", result.Edges)
	}
	if result.Stats.AnalyzedSources != 1 {
		t.Fatalf("unrelated sources fully parsed: %+v", result.Stats)
	}
}

func TestReviewReplacementUsesDestinationVersionSeries(t *testing.T) {
	result := runFixture(t, map[string]string{
		"fork/go.mod": "module example.com/fork\ngo 1.26\n",
		"fork/f.go":   "package fork\nfunc Target(){}",
		"app/go.mod":  "module example.com/app\ngo 1.26\nrequire example.com/original v0.8.0\nreplace example.com/original v0.8.0 => example.com/fork v1.4.0\n",
		"app/p.go":    "package app\nimport original \"example.com/original\"\nfunc Caller(){original.Target()}",
	}, "example.com/fork.Target", Options{})
	edge := regressionEdge(t, result, "example.com/app.Caller", "example.com/fork.Target")
	if edge.Resolution.Status != Resolved || edge.Compatibility.Status != Compatible {
		t.Fatalf("replacement version not respected: %+v", edge)
	}
}
