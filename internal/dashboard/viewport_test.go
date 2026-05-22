package dashboard

import (
	"strings"
	"testing"
)

func TestNewScrollableViewport_DefaultHeight(t *testing.T) {
	v := NewScrollableViewport()
	if v == nil {
		t.Fatal("expected non-nil viewport")
	}
	if v.height != 10 {
		t.Errorf("default height = %d, want 10", v.height)
	}
}

func TestScrollableViewport_SetContent(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(3)

	v.SetContent("a\nb\nc\nd\ne")
	if v.totalLines != 5 {
		t.Errorf("totalLines = %d, want 5", v.totalLines)
	}
	if len(v.lines) != 5 {
		t.Errorf("lines = %d, want 5", len(v.lines))
	}
}

func TestScrollableViewport_SetContent_Empty(t *testing.T) {
	v := NewScrollableViewport()
	v.SetContent("")
	if v.totalLines != 0 {
		t.Errorf("expected empty totalLines, got %d", v.totalLines)
	}
	if v.lines != nil {
		t.Errorf("expected nil lines for empty content, got %#v", v.lines)
	}
}

func TestScrollableViewport_SetContent_ClampsOffset(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(2)
	v.SetContent("a\nb\nc\nd\ne")
	v.GotoBottom() // offset = 3
	v.SetContent("a\nb")
	if v.offset != 0 {
		t.Errorf("offset should clamp after shrink, got %d", v.offset)
	}
}

func TestScrollableViewport_SetHeight_MinimumOne(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(0)
	if v.height != 1 {
		t.Errorf("height should clamp to 1, got %d", v.height)
	}
	v.SetHeight(-5)
	if v.height != 1 {
		t.Errorf("negative height should clamp to 1, got %d", v.height)
	}
}

func TestScrollableViewport_Scrolling(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(2)
	v.SetContent("a\nb\nc\nd\ne") // 5 lines

	if !v.AtTop() {
		t.Error("should start at top")
	}

	v.ScrollDown()
	if v.offset != 1 {
		t.Errorf("after one down, offset = %d, want 1", v.offset)
	}

	v.ScrollUp()
	if v.offset != 0 {
		t.Errorf("after up, offset = %d, want 0", v.offset)
	}

	v.ScrollUp() // should clamp at 0
	if v.offset != 0 {
		t.Errorf("up at top should stay 0, got %d", v.offset)
	}

	v.PageDown() // height=2 → offset=2
	if v.offset != 2 {
		t.Errorf("pagedown offset = %d, want 2", v.offset)
	}

	v.PageDown() // → offset=3 (maxOffset = 5-2 = 3)
	if v.offset != 3 {
		t.Errorf("second pagedown offset = %d, want 3", v.offset)
	}

	if !v.AtBottom() {
		t.Error("should be at bottom")
	}

	v.PageUp() // → offset=1
	if v.offset != 1 {
		t.Errorf("pageup offset = %d, want 1", v.offset)
	}

	v.GotoTop()
	if v.offset != 0 || !v.AtTop() {
		t.Errorf("goto top failed: offset=%d", v.offset)
	}

	v.GotoBottom()
	if v.offset != 3 || !v.AtBottom() {
		t.Errorf("goto bottom failed: offset=%d", v.offset)
	}
}

func TestScrollableViewport_View(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(2)
	v.SetContent("a\nb\nc\nd")

	got := v.View()
	if got != "a\nb" {
		t.Errorf("view at top = %q, want %q", got, "a\nb")
	}

	v.ScrollDown()
	got = v.View()
	if got != "b\nc" {
		t.Errorf("view after scroll = %q, want %q", got, "b\nc")
	}
}

func TestScrollableViewport_View_Empty(t *testing.T) {
	v := NewScrollableViewport()
	if v.View() != "" {
		t.Errorf("empty viewport View() should be empty, got %q", v.View())
	}
}

func TestScrollableViewport_View_HeightExceedsContent(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(10)
	v.SetContent("a\nb")
	got := v.View()
	if got != "a\nb" {
		t.Errorf("view with tall viewport = %q, want %q", got, "a\nb")
	}
}

func TestScrollableViewport_ScrollIndicator(t *testing.T) {
	v := NewScrollableViewport()
	if v.ScrollIndicator() != "[0/0]" {
		t.Errorf("empty indicator = %q, want [0/0]", v.ScrollIndicator())
	}

	v.SetHeight(2)
	v.SetContent("a\nb\nc")
	if v.ScrollIndicator() != "[1/3]" {
		t.Errorf("indicator at top = %q, want [1/3]", v.ScrollIndicator())
	}

	v.ScrollDown()
	if v.ScrollIndicator() != "[2/3]" {
		t.Errorf("indicator after scroll = %q", v.ScrollIndicator())
	}
}

func TestScrollableViewport_MaxOffset_NegativeClampsToZero(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(10)
	v.SetContent("a\nb") // height>content → maxOffset = -8 → 0
	if v.maxOffset() != 0 {
		t.Errorf("maxOffset must clamp to 0 when height exceeds content, got %d", v.maxOffset())
	}
}

func TestScrollableViewport_ClampOffset(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(2)
	v.SetContent("a\nb\nc\nd\ne") // maxOffset = 3
	v.offset = 99
	v.clampOffset()
	if v.offset != 3 {
		t.Errorf("clamp high = %d, want 3", v.offset)
	}
	v.offset = -5
	v.clampOffset()
	if v.offset != 0 {
		t.Errorf("clamp low = %d, want 0", v.offset)
	}
}

func TestScrollableViewport_AtTopAtBottom_EmptyContent(t *testing.T) {
	v := NewScrollableViewport()
	if !v.AtTop() {
		t.Error("empty viewport must be at top")
	}
	if !v.AtBottom() {
		t.Error("empty viewport must also be at bottom (no scroll possible)")
	}
}

func TestScrollableViewport_RealisticLargeContent(t *testing.T) {
	v := NewScrollableViewport()
	v.SetHeight(5)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	v.SetContent(strings.Join(lines, "\n"))
	v.GotoBottom()
	if !v.AtBottom() {
		t.Error("should be at bottom after GotoBottom")
	}
	view := v.View()
	if strings.Count(view, "\n") != 4 {
		t.Errorf("expected 5 visible lines (4 newlines), got %d", strings.Count(view, "\n"))
	}
}
