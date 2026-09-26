// Package output formats reverse call graphs for CLI users.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/meian/rev-callgraph/internal/analysis"
)

// SupportedFormat reports whether the CLI can write a format.
func SupportedFormat(format string) bool {
	switch format {
	case "tree", "json", "dot":
		return true
	default:
		return false
	}
}

// Write renders one result. JSON styles other than edges retain the historical
// nested output behavior.
func Write(w io.Writer, result *analysis.Result, format, jsonStyle string) error {
	if result == nil || result.Root == nil {
		return fmt.Errorf("empty call graph")
	}
	switch format {
	case "tree":
		return writeTree(w, result.Root, 0)
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if jsonStyle == "edges" {
			return encoder.Encode(edgeDocument(result))
		}
		return encoder.Encode(nestedNode(result.Root))
	case "dot":
		return writeDOT(w, result)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

type metadata struct {
	Resolution    *analysis.Resolution    `json:"resolution,omitempty"`
	Compatibility *analysis.Compatibility `json:"compatibility,omitempty"`
}

func edgeMetadata(edge *analysis.Edge) metadata {
	if edge == nil || edge.Resolution.Status == analysis.Resolved && edge.Compatibility.Status == analysis.Compatible {
		return metadata{}
	}
	return metadata{Resolution: &edge.Resolution, Compatibility: &edge.Compatibility}
}

type nested struct {
	Name    string   `json:"name"`
	Callers []nested `json:"callers,omitempty"`
	Cycled  bool     `json:"cycled,omitempty"`
	Main    bool     `json:"main,omitempty"`
	metadata
}

func nestedNode(node *analysis.Node) nested {
	result := nested{Name: node.Name, Cycled: node.Cycle, Main: node.Main, metadata: edgeMetadata(node.Edge)}
	for _, caller := range sortedCallers(node.Callers) {
		result.Callers = append(result.Callers, nestedNode(caller))
	}
	return result
}

type edgeItem struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
	metadata
}

type edges struct {
	Root  string     `json:"root"`
	Nodes []string   `json:"nodes"`
	Edges []edgeItem `json:"edges"`
}

func edgeDocument(result *analysis.Result) edges {
	document := edges{Root: result.Root.Name, Nodes: []string{}, Edges: []edgeItem{}}
	names := map[string]struct{}{result.Root.Name: {}}
	for _, edge := range sortedEdges(result.Edges) {
		names[edge.Caller] = struct{}{}
		names[edge.Callee] = struct{}{}
		document.Edges = append(document.Edges, edgeItem{Caller: edge.Caller, Callee: edge.Callee, metadata: edgeMetadata(&edge)})
	}
	for name := range names {
		document.Nodes = append(document.Nodes, name)
	}
	sort.Strings(document.Nodes)
	return document
}

func writeTree(w io.Writer, node *analysis.Node, depth int) error {
	line := strings.Repeat("  ", depth) + node.Name
	if node.Main {
		line += " [main]"
	}
	if node.Cycle {
		line += " (cycled)"
	}
	if edge := node.Edge; edge != nil && !(edge.Resolution.Status == analysis.Resolved && edge.Compatibility.Status == analysis.Compatible) {
		line += fmt.Sprintf(" [resolution=%s, compatibility=%s]", edge.Resolution.Status, edge.Compatibility.Status)
		if len(edge.Compatibility.Issues) > 0 {
			issues := make([]string, 0, len(edge.Compatibility.Issues))
			for _, issue := range edge.Compatibility.Issues {
				issues = append(issues, issue.Kind)
			}
			line += " (" + strings.Join(issues, ", ") + ")"
		}
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}
	for _, caller := range sortedCallers(node.Callers) {
		if err := writeTree(w, caller, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func writeDOT(w io.Writer, result *analysis.Result) error {
	if _, err := fmt.Fprintln(w, "digraph rev_callgraph {"); err != nil {
		return err
	}
	document := edgeDocument(result)
	for _, name := range document.Nodes {
		if _, err := fmt.Fprintf(w, "  %s;\n", strconv.Quote(name)); err != nil {
			return err
		}
	}
	for _, edge := range sortedEdges(result.Edges) {
		attribute := ""
		if !(edge.Resolution.Status == analysis.Resolved && edge.Compatibility.Status == analysis.Compatible) {
			label := fmt.Sprintf("%s / %s", edge.Resolution.Status, edge.Compatibility.Status)
			attribute = " [label=" + strconv.Quote(label) + "]"
		}
		if _, err := fmt.Fprintf(w, "  %s -> %s%s;\n", strconv.Quote(edge.Caller), strconv.Quote(edge.Callee), attribute); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "}")
	return err
}

func sortedCallers(callers []*analysis.Node) []*analysis.Node {
	result := append([]*analysis.Node(nil), callers...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func sortedEdges(edges []analysis.Edge) []analysis.Edge {
	result := append([]analysis.Edge(nil), edges...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Caller != result[j].Caller {
			return result[i].Caller < result[j].Caller
		}
		return result[i].Callee < result[j].Callee
	})
	return result
}
