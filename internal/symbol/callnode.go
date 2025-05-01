package symbol

// CallNode は呼び出し元ツリーのノードを表す
type CallNode struct {
	// Name は関数名を表す
	Name string `json:"name"`
	// Callers は呼び出し元ノードのスライスを表す
	Callers []*CallNode `json:"callers,omitempty"`
	// Cycled はサイクル到達時にtrueとなる
	Cycled bool `json:"cycled,omitempty"`
	// Main はmainパッケージかどうかを表す
	Main bool `json:"main,omitempty"`
}
