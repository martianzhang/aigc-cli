//go:build tools

// Package tools 锁定仅编译期/构建期使用的模块依赖。
//
// `go mod tidy` 会删除不被 Go 代码 import 的模块。sherpa-onnx-go 及其
// -linux/-macos/-windows 平台模块仅被 scripts/build-helper.sh 编译期使用
// （提供 c-api.h 头文件与预编译动态库），不参与 aigc-cli 运行时，属于此类。
//
// `//go:build tools` 使本文件默认不参与编译，但 go mod tidy 会扫描其
// import 图，从而保留以下模块。新增仅构建期使用的模块时在此空导入锁定。
package tools

import (
	_ "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)
