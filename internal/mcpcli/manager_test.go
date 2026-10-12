package mcpcli

import (
	"context"
	"testing"
)

func TestManagerNilSafe(t *testing.T) {
	var m *Manager
	if m.Tools() != nil {
		t.Error("nil manager Tools() should be nil")
	}
	if m.Has("x") {
		t.Error("nil manager Has() should be false")
	}
	if _, err := m.Call(context.Background(), "x", "{}"); err == nil {
		t.Error("nil manager Call() should error")
	}
	if err := m.Close(); err != nil {
		t.Errorf("nil manager Close() = %v", err)
	}
}

func TestNewManagerNilConfig(t *testing.T) {
	m, err := NewManager(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewManager(nil): %v", err)
	}
	if len(m.Tools()) != 0 {
		t.Errorf("Tools() = %d, want 0", len(m.Tools()))
	}
	if m.Has("generate_image") {
		t.Error("empty manager should not know any tool")
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
