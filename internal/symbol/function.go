package symbol

// Function は追跡対象の関数またはメソッドを表します
type Function struct {
	// PkgPath はパッケージパスを表します
	PkgPath string
	// TypeName は型名を表します
	TypeName string
	// Name は関数名またはメソッド名を表します
	Name string
}

// String はFunctionを文字列に変換します
func (f Function) String() string {
	if f.TypeName != "" {
		return f.PkgPath + "." + f.TypeName + "#" + f.Name
	}
	return f.PkgPath + "." + f.Name
}

// IsMethod はFunctionがメソッドであるかを判定します
func (f Function) IsMethod() bool {
	return f.TypeName != ""
}
