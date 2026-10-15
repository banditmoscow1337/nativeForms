package nativeforms

import "testing"

func TestFontLayoutCacheBoundAndCallerOwnership(t *testing.T) {
	ctx := NewFontContext()
	spec := FontSpec{Size: 14}
	first := ctx.Layout("layout cache", spec)
	first.Carets[0] = 12345
	second := ctx.Layout("layout cache", spec)
	if second.Carets[0] == 12345 {
		t.Fatal("caller changed cached caret positions")
	}
	bytes, entries, hits, misses := ctx.LayoutCacheStats()
	if bytes <= 0 || entries != 1 || hits != 1 || misses != 1 {
		t.Fatalf("cache stats: %d %d %d %d", bytes, entries, hits, misses)
	}
	ctx.SetLayoutCacheLimit(0)
	bytes, entries, _, _ = ctx.LayoutCacheStats()
	if bytes != 0 || entries != 0 {
		t.Fatal("layout cache did not evict")
	}
}

func TestFontAdvanceOnlyMatchesShapedLayout(t *testing.T) {
	for _, sample := range []string{"AV AV", "Е\u0308", "А\u0301", "\tA", "unknown", "A\u200bV"} {
		ctx := stageFiveTestContext()
		spec := FontSpec{Family: "primary", Size: 16}
		got := ctx.Advance(sample, spec)
		if bytes, _ := ctx.CacheUsage(); bytes != 0 {
			t.Fatalf("%q: measuring offscreen text rasterized %d glyph bytes", sample, bytes)
		}
		want := ctx.Layout(sample, spec).Advance
		if !nearStageFive(got, want) {
			t.Fatalf("%q: advance %v, layout %v", sample, got, want)
		}
	}
}
