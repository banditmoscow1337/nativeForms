package render

import (
	"image"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func TestGlyphAtlasPackingAndStableUpdates(t *testing.T) {
	atlas := NewGlyphAtlas(8)
	a := image.NewAlpha(image.Rect(3, 4, 5, 6))
	b := image.NewAlpha(image.Rect(0, 0, 2, 2))
	ra, ok := atlas.Place(a)
	if !ok || ra != image.Rect(1, 1, 3, 3) {
		t.Fatalf("first placement = %v, %v", ra, ok)
	}
	if again, ok := atlas.Place(a); !ok || again != ra || atlas.UpdateCount() != 1 {
		t.Fatal("repeat mask must reuse its atlas region without uploading again")
	}
	rb, ok := atlas.Place(b)
	if !ok || !ra.Intersect(rb).Empty() || atlas.UpdateCount() != 2 {
		t.Fatalf("second placement = %v, %v", rb, ok)
	}
	if got := atlas.UpdatesSince(1); len(got) != 1 || got[0].Mask != b {
		t.Fatalf("incremental updates = %+v", got)
	}
	oversized := image.NewAlpha(image.Rect(0, 0, 8, 8))
	if _, ok := atlas.Place(oversized); ok {
		t.Fatal("oversized mask should use geometry fallback")
	}
}

func TestBuildWithAtlasPreservesOrderAndClipping(t *testing.T) {
	mask := image.NewAlpha(image.Rect(0, 0, 2, 2))
	mask.Pix[0] = 128
	frame := ui.Frame{Width: 10, Height: 10, Commands: []ui.DrawCommand{
		{Kind: ui.CommandQuad, Rect: ui.Rect{W: 10, H: 10}, Clip: ui.Rect{W: 10, H: 10}, Tint: ui.RGBA(255, 255, 255, 255)},
		{Kind: ui.CommandMask, Mask: mask, Rect: ui.Rect{X: 1, Y: 1, W: 4, H: 4},
			Clip: ui.Rect{X: 2, Y: 2, W: 2, H: 2}, Tint: ui.RGBA(255, 0, 0, 255)},
		{Kind: ui.CommandMask, Mask: mask, Rect: ui.Rect{X: 5, Y: 1, W: 4, H: 4},
			Clip: ui.Rect{W: 10, H: 10}, Tint: ui.RGBA(255, 0, 0, 255)},
	}}
	atlas := NewGlyphAtlas(16)
	mesh, err := BuildFrame(frame, Mesh{}, atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(mesh.Batches) != 2 || mesh.Batches[0].Texture != (TextureKey{}) ||
		mesh.Batches[1].Texture != MaskTexture || mesh.Batches[1].Count != 12 || atlas.UpdateCount() != 1 {
		t.Fatalf("unexpected batches %+v, updates %d", mesh.Batches, atlas.UpdateCount())
	}
	if got := mesh.Vertices[6]; got.X != 2 || got.Y != 2 || got.U != 1.5/16 || got.V != 1.5/16 {
		t.Fatalf("clipped mask vertex %+v", got)
	}
	if _, err := BuildFrame(ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{Kind: 255}}}, mesh, atlas); err == nil {
		t.Fatal("invalid command was silently dropped")
	}
}
