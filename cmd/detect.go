package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var detectCmd = &cobra.Command{
	Use:          "detect <file...>",
	Short:        "Detect watermarks, metadata, and AIGC signals in images",
	SilenceUsage: true,
	Long: `AIGC 检测与研究工具 — 通过多信号融合分析图片是否为 AI 生成。

分析信号包括:
  - C2PA Content Credentials（防篡改溯源元数据）
  - TC260 AIGC 标签（国标 GB 45438-2025）
  - SynthID 隐形水印（从 C2PA 厂商推断）
  - FFT 频谱分析（像素级频域伪影）
  - ONNX 模型推理（需下载模型）
  - 可见 AI 水印检测（需通过 --learn-watermark 学习后方可检测）

所有信号融合为单一 AIGen 置信度评分（含 emoji）。

⚠️ 合规声明
本项目不内置任何厂商的水印去除能力。--remove-watermark 仅对用户
通过 --learn-watermark 自行学习的水印生效。用户应自行确保使用
行为符合适用法律法规。

--add-watermark 仅用于为去水印算法创建测试样本，不注入任何元数据。

支持 PNG、JPEG、WebP、GIF、BMP 格式。`,
	RunE: runDetect,
}

var detectJSON bool
var detectPreview bool
var detectRemoveWM bool
var detectAddWM bool
var detectWmProducer string
var detectLearnWM string          // --learn-watermark {name}
var detectLearnStrategy string    // --strategy for learn-watermark
var detectNativeWidth int         // --native-width for imported alpha maps
var detectMarginXFrac float64     // --margin-x-frac
var detectMarginYFrac float64     // --margin-y-frac
var detectDetectThreshold float64 // --detect-threshold

func runDetect(cmd *cobra.Command, args []string) error {
	// --learn-watermark: learn a custom watermark from seed images
	if detectLearnWM != "" {
		return runLearnWatermark(detectLearnWM)
	}

	if len(args) > 0 {
		return detectFiles(args, "")
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return fmt.Errorf("no files specified: pass file paths as arguments or pipe file data to stdin")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("no data read from stdin")
	}

	tmpFile, err := os.CreateTemp("", "aigc-cli-detect-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	return detectFiles([]string{tmpFile.Name()}, "(stdin)")
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().BoolVar(&detectJSON, "json", false, "output results as JSON")
	detectCmd.Flags().BoolVar(&detectPreview, "preview", false, "open image in system viewer after detection")
	detectCmd.Flags().BoolVar(&detectRemoveWM, "remove-watermark", false, "remove visible AI watermarks learned via --learn-watermark. Requires --producer {name}.")
	detectCmd.Flags().BoolVar(&detectAddWM, "add-watermark", false, "add a visible AI watermark for testing removal (no metadata injected)")
	detectCmd.Flags().StringVar(&detectWmProducer, "producer", "",
		`watermark producer name learned via --learn-watermark, e.g. "gemini"`)
	detectCmd.Flags().StringVar(&detectLearnWM, "learn-watermark", "", `learn a watermark from seed images in ~/.config/aigc-cli/watermark/
  {name}.black.png + {name}.gray.png (single pair)
  {name}.2.black.png + {name}.2.gray.png (2nd pair, averaged for lower noise)
  {name}.3.black.png + ... (any number of pairs, all averaged) `)
	detectCmd.Flags().StringVar(&detectLearnStrategy, "strategy", "alpha_blend",
		`removal strategy: "alpha_blend" (default, reverse alpha blending) or "inpaint" (texture fill for badge-type watermarks)`)
	detectCmd.Flags().IntVar(&detectNativeWidth, "native-width", 1024,
		"native image width the alpha map was calibrated at (for imported alpha maps)")
	detectCmd.Flags().Float64Var(&detectMarginXFrac, "margin-x-frac", 0.02,
		"right margin fraction of image width (for imported alpha maps)")
	detectCmd.Flags().Float64Var(&detectMarginYFrac, "margin-y-frac", 0.02,
		"bottom margin fraction of image width (for imported alpha maps)")
	detectCmd.Flags().Float64Var(&detectDetectThreshold, "detect-threshold", 0.25,
		"minimum NCC confidence for detection (for imported alpha maps)")
}
