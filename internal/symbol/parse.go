package symbol

import (
	"fmt"
	"strings"
)

// ParseFunction はターゲット指定文字列をFunction構造体にパースします
func ParseFunction(target string) (Function, error) {
	sepIdx := strings.LastIndex(target, ".")
	if sepIdx < 0 || sepIdx+1 >= len(target) {
		return Function{}, fmt.Errorf("targetのパッケージ区切りが不正: %s", target)
	}
	pkgPath := target[:sepIdx]
	if pkgPath == "" {
		return Function{}, fmt.Errorf("targetのパッケージパスが空: %s", target)
	}
	rest := target[sepIdx+1:]
	if rest == "" {
		return Function{}, fmt.Errorf("targetの関数/メソッド名が空: %s", target)
	}
	var typeName, name string
	if hashIdx := strings.Index(rest, "#"); hashIdx >= 0 {
		if hashIdx == 0 || hashIdx+1 >= len(rest) {
			return Function{}, fmt.Errorf("targetの型名またはメソッド名が不正: %s", target)
		}
		typeName = rest[:hashIdx]
		name = rest[hashIdx+1:]
		if typeName == "" || name == "" {
			return Function{}, fmt.Errorf("targetの型名またはメソッド名が空: %s", target)
		}
	} else {
		typeName = ""
		name = rest
		if name == "" {
			return Function{}, fmt.Errorf("targetの関数名が空: %s", target)
		}
	}
	return Function{
		PkgPath:  pkgPath,
		TypeName: typeName,
		Name:     name,
	}, nil
}
