package grep

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SearchFiles は root 以下の .go ファイルを走査し、
// target 文字列を含むファイルのパス一覧を返します。
// メソッド指定の場合 '#' と '.' の両方で検索します。
func SearchFiles(ctx context.Context, root, target string) ([]string, error) {
	// 検索パターンを準備
	patterns := []string{target}
	// メソッドの場合は#を.に置換
	if strings.Contains(target, "#") {
		dot := strings.ReplaceAll(target, "#", ".")
		patterns = append(patterns, dot)
	}
	// 関数名のみ (例: Target)
	base := target
	if i := strings.LastIndexAny(target, ".#"); i >= 0 {
		base = target[i+1:]
	}
	patterns = append(patterns, base+"(")

	var results []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// ディレクトリのスキップ
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		// .go ファイルのみ
		if filepath.Ext(path) != ".go" {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			for _, p := range patterns {
				if strings.Contains(line, p) {
					results = append(results, path)
					return nil
				}
			}
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
