package ui

import (
	"bytes"
	"os"
	"testing"
)

func TestSetOutOverridesAndRestores(t *testing.T) {
	if Out() != os.Stdout {
		t.Fatal("Out() should default to the live os.Stdout")
	}
	var buf bytes.Buffer
	restore := SetOut(&buf)
	Printf("hello %s", "ui")
	Println("!")
	if got := buf.String(); got != "hello ui!\n" {
		t.Fatalf("out = %q", got)
	}
	restore()
	if Out() != os.Stdout {
		t.Fatal("restore must return Out() to os.Stdout")
	}
}

func TestSetErrOverrides(t *testing.T) {
	var buf bytes.Buffer
	restore := SetErr(&buf)
	Eprintf("bad %d", 7)
	restore()
	if got := buf.String(); got != "bad 7" {
		t.Fatalf("err = %q", got)
	}
}
