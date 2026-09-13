package detect

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/service"
)

func TestModelFilename_vitBase(t *testing.T) {
	got := modelFilename("vit-base")
	if got != "model-vit-base.onnx" {
		t.Errorf("modelFilename('vit-base') = %q, want 'model-vit-base.onnx'", got)
	}
}

func TestModelFilename_distilledVit(t *testing.T) {
	got := modelFilename("distilled-vit")
	if got != "model-distilled-vit.onnx" {
		t.Errorf("modelFilename('distilled-vit') = %q, want 'model-distilled-vit.onnx'", got)
	}
}

func TestModelFilename_unknown(t *testing.T) {
	got := modelFilename("nonexistent")
	if got != "model-vit-base.onnx" {
		t.Errorf("modelFilename('nonexistent') = %q, want default 'model-vit-base.onnx'", got)
	}
}

func TestModelSizeLabel_vitBase(t *testing.T) {
	got := modelSizeLabel("/some/path/model-vit-base.onnx")
	if got != "vit-base" {
		t.Errorf("modelSizeLabel() = %q, want 'vit-base'", got)
	}
}

func TestModelSizeLabel_distilledVit(t *testing.T) {
	got := modelSizeLabel("/some/path/model-distilled-vit.onnx")
	if got != "distilled-vit" {
		t.Errorf("modelSizeLabel() = %q, want 'distilled-vit'", got)
	}
}

func TestModelSizeLabel_unknown(t *testing.T) {
	got := modelSizeLabel("/some/path/other-model.onnx")
	if got != "other-model.onnx" {
		t.Errorf("modelSizeLabel() = %q, want 'other-model.onnx'", got)
	}
}

func TestSafeC2PAVendor_nil(t *testing.T) {
	if got := safeC2PAVendor(nil); got != "" {
		t.Errorf("safeC2PAVendor(nil) = %q, want ''", got)
	}
}

func TestSafeC2PAVendor_notNil(t *testing.T) {
	r := &service.C2PAResult{Vendor: "OpenAI"}
	if got := safeC2PAVendor(r); got != "OpenAI" {
		t.Errorf("safeC2PAVendor() = %q, want 'OpenAI'", got)
	}
}

func TestSafeC2PASource_nil(t *testing.T) {
	if got := safeC2PASource(nil); got != "" {
		t.Errorf("safeC2PASource(nil) = %q, want ''", got)
	}
}

func TestSafeTC260Provider_nil(t *testing.T) {
	if got := safeTC260Provider(nil); got != "" {
		t.Errorf("safeTC260Provider(nil) = %q, want ''", got)
	}
}

func TestSafeSynthIDSource_nil(t *testing.T) {
	if got := safeSynthIDSource(nil); got != "" {
		t.Errorf("safeSynthIDSource(nil) = %q, want ''", got)
	}
}

func TestSafeCameraMake_nil(t *testing.T) {
	if got := safeCameraMake(nil); got != "" {
		t.Errorf("safeCameraMake(nil) = %q, want ''", got)
	}
}

func TestSafeCameraModel_nil(t *testing.T) {
	if got := safeCameraModel(nil); got != "" {
		t.Errorf("safeCameraModel(nil) = %q, want ''", got)
	}
}

func TestSafeCameraMake_notNil(t *testing.T) {
	r := &service.CameraInfo{Make: "Canon", Model: "EOS R5"}
	if got := safeCameraMake(r); got != "Canon" {
		t.Errorf("safeCameraMake() = %q, want 'Canon'", got)
	}
	if got := safeCameraModel(r); got != "EOS R5" {
		t.Errorf("safeCameraModel() = %q, want 'EOS R5'", got)
	}
}
