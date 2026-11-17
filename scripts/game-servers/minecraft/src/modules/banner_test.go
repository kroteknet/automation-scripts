package modules

import (
	"strings"
	"testing"

	"krotek-minecraft-installer/core"
)

func TestGetLiveBanner_NoDoublePercentAndFitsWidth(t *testing.T) {
	for _, w := range []int{60, 80, 103, 130} {
		lines := GetLiveBanner(w)
		if len(lines) == 0 {
			t.Fatalf("GetLiveBanner(%d) returned empty slice", w)
		}
		for i, l := range lines {
			clean := core.StripANSI(l)
			if strings.Contains(clean, "%%") {
				t.Fatalf("GetLiveBanner(%d) line %d contains %% literal: %q", w, i, clean)
			}
			if rw := core.RuneWidth(l); rw > w {
				t.Fatalf("GetLiveBanner(%d) line %d width %d > %d: %q", w, i, rw, w, clean)
			}
		}
	}
}
