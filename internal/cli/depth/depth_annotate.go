package depth

import (
	"fmt"
	"image"

	"github.com/martianzhang/aigc-cli/internal/annotate"
)

// annotateSkeleton detects a human pose and overlays it onto the depth image.
func annotateSkeleton(input, output string) error {
	if err := annotate.AnnotateImage(input, output, output, annotate.Options{Skeleton: true}); err != nil {
		return err
	}
	fmt.Printf("Skeleton annotated on depth: %s\n", output)
	return nil
}

// annotateFace detects faces (pure-Go pigo) and overlays them onto the depth image.
func annotateFace(input, output string) error {
	if err := annotate.AnnotateImage(input, output, output, annotate.Options{Face: true}); err != nil {
		return err
	}
	fmt.Printf("Face annotated on depth: %s\n", output)
	return nil
}

// annotateVideoOptions carries the parameters for video annotation.
type annotateVideoOptions struct {
	skeleton bool
	face     bool
}

// newAnnotateVideoCallback creates the depth.ConvertOptions.Annotate callback.
func newAnnotateVideoCallback(opts annotateVideoOptions) (func(framePath string, gray *image.Gray) ([]uint8, bool), func(), error) {
	return annotate.AnnotateVideoCallback(annotate.Options{
		Skeleton: opts.skeleton,
		Face:     opts.face,
	})
}
