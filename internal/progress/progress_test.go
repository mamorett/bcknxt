package progress

import (
	"testing"
)

func TestProgressBar(t *testing.T) {
	pb := NewProgressBar(5, "Test Task")
	defer pb.Finish()

	Log("Log message 1")
	pb.Describe("Step 1")
	pb.UpdateCopy("file1.txt", "/tmp/file1.txt")

	Log("Log message 2")
	pb.Describe("Step 2")
	pb.UpdateCopy("file2.txt", "/tmp/file2.txt")

	if pb.copied != 2 {
		t.Errorf("expected copied 2, got %d", pb.copied)
	}
}
