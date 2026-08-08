package depth

import (
	"runtime"
	"testing"
)

func TestOptimalThreadsWithinBounds(t *testing.T) {
	n := optimalThreads()
	if n <= 0 {
		t.Fatalf("optimalThreads() = %d, want > 0", n)
	}
	if n > runtime.NumCPU() {
		t.Errorf("optimalThreads() = %d exceeds NumCPU = %d", n, runtime.NumCPU())
	}
}

func TestOptimalThreadsNonZero(t *testing.T) {
	if n := optimalThreads(); n < 1 {
		t.Errorf("optimalThreads() = %d, want >= 1", n)
	}
}

func TestParallelismCountBounds(t *testing.T) {
	n := parallelismCount()
	if n <= 0 {
		t.Fatalf("parallelismCount() = %d, want > 0", n)
	}
	if n > 4 {
		t.Errorf("parallelismCount() = %d, want <= 4 (memory cap)", n)
	}
}
