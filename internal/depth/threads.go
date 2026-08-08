package depth

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// optimalThreads 返回 ONNX Runtime intra-op 线程数的自适应配置。
//
// 实测（Apple M4, 4 性能核 + 6 能效核, 10 逻辑核）：
//   - 2 线程 694ms, 4 线程 484ms, 8 线程 445ms（最优）, 10 线程 487ms, 12 线程 750ms
//
// 规律：最优 ≈ 性能核数 × 2；超过逻辑核数（超线程）反而变慢（线程竞争）。
// 策略：优先用平台 API 拿性能核数（macOS），否则用逻辑核数/2 估算。
func optimalThreads() int {
	logical := runtime.NumCPU()
	if logical <= 0 {
		return 4
	}

	perf := perfCoreCount()
	if perf <= 0 {
		// 无性能核信息（Intel/AMD）：逻辑核/2 ≈ 物理核，再 ×2 = 逻辑核。
		// 留一个线程给主流程，避免全核竞争。
		perf = logical / 2
	}

	n := perf * 2
	if n > logical {
		n = logical
	}
	if n < 1 {
		n = 1
	}
	return n
}

// perfCoreCount 返回 macOS 性能核数量；其他平台返回 0。
func perfCoreCount() int {
	if runtime.GOOS != "darwin" {
		return 0
	}
	out, err := exec.Command("sysctl", "-n", "hw.perflevel0.logicalcpu").Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
