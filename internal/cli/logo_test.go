package cli

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

func TestPrintTransparentHalfblocks(t *testing.T) {
	srcImg, _, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		t.Fatalf("decode err: %v", err)
	}
	var buf bytes.Buffer
	PrintTransparentHalfblocksTo(&buf, srcImg, 50, 25)
	output := buf.String()
	t.Logf("Output length: %d bytes, non-space count: %d", len(output), strings.Count(output, "▀")+strings.Count(output, "▄"))
}

func TestPrintLogo(t *testing.T) {
	PrintLogo()
}
