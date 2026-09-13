package cmd

import (
	"fmt"
	"os"
	"sync"

	"github.com/martianzhang/aigc-cli/internal/onnx"
	"github.com/martianzhang/aigc-cli/internal/wmremove"
)

var wmDetector *wmremove.Detector

// detectOnce lazily initializes the ONNX detector and caches the instance
// for reuse across detect files, avoiding repeated model loading.
var detectOnce struct {
	sync.Once
	detector *onnx.Detector
}

func detectFiles(paths []string, pathOverride string) error {
	if detectOnce.detector == nil {
		detectOnce.Do(func() {
			detectOnce.detector = tryInitONNX()
		})
	}
	aiDetector := detectOnce.detector
	if aiDetector != nil {
		defer aiDetector.Close()
	}
	if detectRemoveWM {
		if d, err := tryInitWMRemove(); err == nil {
			wmDetector = d
		}
	}

	if detectJSON {
		return detectFilesJSON(paths, pathOverride, aiDetector)
	}

	for _, path := range paths {
		if err := detectOneFile(path, pathOverride, aiDetector); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}
