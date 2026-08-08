package onnxrt

import (
	"fmt"
	"sync"

	ort "github.com/amikos-tech/pure-onnx/ort"
)

// envOnce 保证 ONNX Runtime 环境进程内只初始化一次。
// depth/skeleton/face 等多个 Detector 共享同一全局环境；pure-onnx 在
// SetSharedLibraryPath 之后不允许重复设置（refCount 守卫），因此所有
// Detector 初始化必须走这里。
var envOnce sync.Once
var envErr error

// InitEnvironment 初始化 ONNX Runtime 全局环境（进程内只执行一次）。
// 返回 nil 表示环境就绪（可能是本次初始化或先前已初始化）。
func InitEnvironment(libPath string) error {
	envOnce.Do(func() {
		envErr = ort.SetSharedLibraryPath(libPath)
		if envErr == nil {
			_ = ort.SetLogLevel(ort.LoggingLevelError)
			envErr = ort.InitializeEnvironment()
		}
	})
	if envErr != nil {
		return fmt.Errorf("initialize ort environment: %w", envErr)
	}
	return nil
}
