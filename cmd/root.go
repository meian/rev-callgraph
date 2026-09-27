package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/meian/rev-callgraph/internal/analysis"
	"github.com/meian/rev-callgraph/internal/output"
	"github.com/spf13/cobra"
)

// Execute runs the command line application.
func Execute() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var dir, format, jsonStyle, symbolSet, goos, goarch string
	var maxDepth int
	var progress bool

	command := &cobra.Command{
		Use:           "rev-callgraph <target>",
		Short:         "逆方向コールグラフ生成ツール",
		Long:          "Goコードの逆方向コールグラフを生成するCLIツールです。",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, args []string) error {
			if symbolSet != string(analysis.Runtime) && symbolSet != string(analysis.Test) {
				return fmt.Errorf("unsupported symbol set: %s", symbolSet)
			}
			if !output.SupportedFormat(format) {
				return fmt.Errorf("unsupported format: %s", format)
			}
			root, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("絶対パスの取得失敗: %w", err)
			}
			if progress {
				fmt.Fprintf(command.ErrOrStderr(), "analyzing %s\n", args[0])
			}
			result, err := analysis.Analyze(command.Context(), args[0], analysis.Options{
				Dir: root, SymbolSet: analysis.SymbolSet(symbolSet),
				Build: analysis.BuildContext{GOOS: goos, GOARCH: goarch}, MaxDepth: maxDepth,
			})
			if err != nil {
				return err
			}
			if progress {
				fmt.Fprintf(command.ErrOrStderr(), "analyzed %d source files\n", result.Stats.AnalyzedSources)
			}
			return output.Write(command.OutOrStdout(), result, format, jsonStyle)
		},
	}
	command.Flags().StringVar(&dir, "dir", ".", "解析するワークスペースのルートディレクトリ")
	command.Flags().StringVar(&format, "format", "tree", "出力形式: json|tree|dot")
	command.Flags().StringVar(&jsonStyle, "json-style", "nested", "json出力スタイル: nested|edges")
	command.Flags().IntVar(&maxDepth, "max-depth", 0, "逆探索の最大深さ (0は制限なし)")
	command.Flags().BoolVar(&progress, "progress", false, "進捗を表示するかどうか")
	command.Flags().StringVar(&symbolSet, "symbol-set", string(analysis.Runtime), "解析対象: runtime|test")
	command.Flags().StringVar(&goos, "goos", "", "解析対象のGOOS (省略時は実行環境)")
	command.Flags().StringVar(&goarch, "goarch", "", "解析対象のGOARCH (省略時は実行環境)")
	return command
}
