package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/meian/rev-callgraph/internal/callgraph"
	"github.com/meian/rev-callgraph/internal/format"
	"github.com/meian/rev-callgraph/internal/gomod"
	"github.com/meian/rev-callgraph/internal/symbol"
	"github.com/spf13/cobra"
)

// rootp はコマンドラインフラグ値を保持します
var rootp struct {
	// Dir は解析するワークスペースのルートディレクトリ
	// デフォルトはカレントディレクトリ
	Dir string
	// Format は出力形式
	Format string
	// JSONStyle はJSON指定時のスタイル
	JSONStyle string
	// MaxDepth は逆探索の最大深さ
	MaxDepth int
}

var rootCmd = &cobra.Command{
	Use:   "rev-callgraph <target>",
	Short: "逆方向コールグラフ生成ツール",
	Long:  `Goコードの逆方向コールグラフを生成するCLIツールです。`,
	Args:  cobra.ExactArgs(1),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		slog.SetDefault(slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)))
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		target := args[0]
		dir := rootp.Dir
		if dir == "" {
			dir = filepath.Clean(".")
		}
		dir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("絶対パスの取得失敗: %w", err)
		}

		// targetパース
		f, err := symbol.ParseFunction(target)
		if err != nil {
			return fmt.Errorf("targetの分解失敗: %w", err)
		}

		// ディレクトリ内の全モジュールを検出
		mods, err := gomod.Scan(ctx, dir)
		if err != nil {
			return fmt.Errorf("モジュールスキャン失敗: %w", err)
		}
		slog.Debug("モジュール検出", "modules", mods)

		// 対象が含まれるモジュールを検出
		mod, err := mods.FindByFunction(ctx, f)
		if err != nil {
			return fmt.Errorf("targetの存在確認失敗: %w", err)
		}
		if mod == nil {
			return fmt.Errorf("targetが見つかりません: %s", target)
		}

		root, err := callgraph.CallersTree(ctx, *mod, target, *mods, 0, nil, rootp.MaxDepth)
		if err != nil {
			return fmt.Errorf("呼び出し元の取得失敗: %w", err)
		}

		p, err := format.NewPrinter(rootp.Format, rootp.JSONStyle)
		if err != nil {
			return err
		}
		return p.Print(root)
	},
}

// Execute はCLIを実行します
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&rootp.Dir, "dir", "", "解析するワークスペースのルートディレクトリ")
	rootCmd.Flags().StringVar(&rootp.Format, "format", "tree", "出力形式: json|tree|dot")
	rootCmd.Flags().StringVar(&rootp.JSONStyle, "json-style", "nested", "json出力スタイル: nested|edges")
	rootCmd.Flags().IntVar(&rootp.MaxDepth, "max-depth", 0, "逆探索の最大深さ (0は制限なし)")
}
