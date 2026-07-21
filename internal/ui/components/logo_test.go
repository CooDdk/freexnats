package components

import (
	"regexp"
	"strings"
	"testing"

	"github.com/CooDdk/freexnats/internal/config"
)

var ansiSequencePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(input string) string {
	return ansiSequencePattern.ReplaceAllString(input, "")
}

func TestPixelLogoRendersReadableBrandName(t *testing.T) {
	logo := stripANSI(PixelLogo())

	// Correct grouping renders as FREEX  |gap|  NATS — the X letter block
	// sits at the end of the first group, one space after the last E block,
	// and a large gap separates it from the following N block.
	for _, want := range []string{
		"EEEEEE X    X",   // X immediately follows the E letter
		"X    X      NN",  // large gap between X block and N block
	} {
		if !strings.Contains(logo, want) {
			t.Fatalf("expected pixel logo to render as FREEX + NATS, missing %q in %q", want, logo)
		}
	}

	// Regression: earlier version rendered as FREE  |gap|  XNATS which put
	// the X block on the wrong side of the gap.
	for _, notWant := range []string{"EEEEEE      X", "X    X NN"} {
		if strings.Contains(logo, notWant) {
			t.Fatalf("pixel logo should not render as FREE + XNATS, found %q in %q", notWant, logo)
		}
	}
}

func TestLogoWithSubtitleIncludesBrandAndMetadata(t *testing.T) {
	logo := stripANSI(LogoWithSubtitle())

	for _, want := range []string{
		"FreeX Nats",
		"──  FreeX Nats  ·  " + config.AppVersion + " ──",
		config.AppDesc,
	} {
		if !strings.Contains(logo, want) {
			t.Fatalf("expected logo block to contain %q, got %q", want, logo)
		}
	}
}
