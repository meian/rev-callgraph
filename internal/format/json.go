// Package format はコールグラフ出力のフォーマットを提供します
package format

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/meian/rev-callgraph/internal/symbol"
)

// jsonPrinter はJSON形式でコールグラフを出力するプリンタです。
type jsonPrinter struct {
	style string // nested or edges
}

func init() {
	printers["json"] = func(style string) Printer {
		return &jsonPrinter{style: style}
	}
}

// Print はコールグラフをJSON形式で出力します。
func (p *jsonPrinter) Print(root *symbol.CallNode) error {
	if root == nil {
		return nil
	}
	var (
		data []byte
		err  error
	)
	switch p.style {
	case "edges":
		edges := buildEdges(root)
		data, err = json.MarshalIndent(edges, "", "  ")
	default:
		data, err = json.MarshalIndent(root, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("JSONエンコード失敗: %w", err)
	}
	_, err = os.Stdout.Write(data)
	if err == nil {
		os.Stdout.Write([]byte("\n"))
	}
	return err
}

// edgesJSON はedges形式の出力構造体です
type edgesJSON struct {
	Root  string              `json:"root"`
	Nodes []string            `json:"nodes"`
	Edges []map[string]string `json:"edges"`
}

// buildEdges はCallNodeツリーからedges形式の構造体を生成します
func buildEdges(root *symbol.CallNode) edgesJSON {
	nodes := make(map[string]struct{})
	edges := make([]map[string]string, 0)
	var walk func(n *symbol.CallNode)
	walk = func(n *symbol.CallNode) {
		if n == nil {
			return
		}
		nodes[n.Name] = struct{}{}
		for _, c := range n.Callers {
			edges = append(edges, map[string]string{
				"caller": c.Name,
				"callee": n.Name,
			})
			walk(c)
		}
	}
	walk(root)
	// ノード名リスト化
	ns := make([]string, 0, len(nodes))
	for k := range nodes {
		ns = append(ns, k)
	}
	return edgesJSON{
		Root:  root.Name,
		Nodes: ns,
		Edges: edges,
	}
}
