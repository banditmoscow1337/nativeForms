package render

import (
	"image"
	"image/color"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/internal/renderparity"
)

func TestFreezeOwnsFrameAndDamage(t *testing.T) {
	mask := image.NewAlpha(image.Rect(0, 0, 2, 1))
	mask.Pix[0] = 80
	frame := ui.Frame{Width: 8, Height: 8, Commands: []ui.DrawCommand{{
		Kind: ui.CommandMask, Mask: mask, Rect: ui.Rect{X: 1, Y: 2, W: 2, H: 1},
		Clip: ui.Rect{W: 8, H: 8}, Tint: ui.RGBA(255, 255, 255, 255),
	}}}
	first, err := Freeze(frame, nil)
	if err != nil || first.Damage() != image.Rect(0, 0, 8, 8) {
		t.Fatalf("first snapshot: %v, %v", first, err)
	}
	frame.Commands[0].Rect.X = 4 // Manager reuses the command backing array.
	mask.Pix[0] = 255
	copyFrame := first.Frame()
	if copyFrame.Commands[0].Rect.X != 1 || copyFrame.Commands[0].Mask.Pix[0] != 80 {
		t.Fatal("snapshot borrowed mutable command or glyph pixels")
	}
	copyFrame.Commands[0].Mask.Pix[0] = 0
	if first.Frame().Commands[0].Mask.Pix[0] != 80 {
		t.Fatal("Frame exposed mutable display-list pixels")
	}
	second, err := Freeze(frame, first)
	if err != nil || second.Damage() != image.Rect(1, 2, 6, 3) {
		t.Fatalf("changed coverage: damage=%v err=%v", second.Damage(), err)
	}
	third, err := Freeze(frame, second)
	if err != nil || !third.Damage().Empty() {
		t.Fatalf("unchanged frame: damage=%v err=%v", third.Damage(), err)
	}
	frame.Commands = nil
	fourth, err := Freeze(frame, third)
	if err != nil || fourth.Damage() != image.Rect(4, 2, 6, 3) {
		t.Fatalf("removed command: damage=%v err=%v", fourth.Damage(), err)
	}
}

func TestImageVersionInvalidatesDamageWithoutChangingOldPixels(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	old, err := ui.NewImageResource(source)
	if err != nil {
		t.Fatal(err)
	}
	frame := ui.Frame{Width: 4, Height: 4, Commands: []ui.DrawCommand{{
		Kind: ui.CommandImage, Image: old, Rect: ui.Rect{X: 1, Y: 1, W: 2, H: 2},
		Clip: ui.Rect{W: 4, H: 4}, Tint: ui.RGBA(255, 255, 255, 255),
	}}}
	first, _ := Freeze(frame, nil)
	source.SetRGBA(0, 0, color.RGBA{B: 255, A: 255})
	updated, err := old.Replace(source)
	if err != nil {
		t.Fatal(err)
	}
	frame.Commands[0].Image = updated
	second, err := Freeze(frame, first)
	if err != nil || second.Damage() != image.Rect(1, 1, 3, 3) ||
		first.Frame().Commands[0].Image.RGBAAt(0, 0).R != 255 {
		t.Fatalf("replacement lost old pixels or damage: %v %v", second.Damage(), err)
	}
}

func TestParityScenesProduceOrderedBatches(t *testing.T) {
	for _, scene := range renderparity.Scenes() {
		t.Run(scene.Name, func(t *testing.T) {
			list, err := Freeze(scene.Frame, nil)
			if err != nil {
				t.Fatal(err)
			}
			mesh, err := Build(list, Mesh{})
			if err != nil {
				t.Fatal(err)
			}
			if len(mesh.Vertices) == 0 || len(mesh.Vertices)%3 != 0 {
				t.Fatalf("scene has no triangles: %d vertices", len(mesh.Vertices))
			}
			covered := 0
			for _, batch := range mesh.Batches {
				if batch.Count <= 0 || batch.First != covered || batch.First+batch.Count > len(mesh.Vertices) {
					t.Fatalf("incorrect batch range %+v", batch)
				}
				if batch.Image != nil && (batch.Texture.ID != batch.Image.ID() || batch.Texture.Version != batch.Image.Version()) {
					t.Fatalf("lost image identity: %+v", batch)
				}
				covered += batch.Count
			}
			if covered != len(mesh.Vertices) {
				t.Fatalf("unbatched geometry: %d/%d", covered, len(mesh.Vertices))
			}
		})
	}
}

func TestImageClipCropAndPainterOrder(t *testing.T) {
	resource, err := ui.NewImageResource(image.NewRGBA(image.Rect(0, 0, 4, 2)))
	if err != nil {
		t.Fatal(err)
	}
	frame := ui.Frame{Width: 10, Height: 10, Commands: []ui.DrawCommand{
		{Kind: ui.CommandQuad, Rect: ui.Rect{W: 10, H: 10}, Clip: ui.Rect{W: 10, H: 10}, Tint: ui.RGBA(255, 0, 0, 255)},
		{Kind: ui.CommandImage, Image: resource, Rect: ui.Rect{W: 8, H: 4}, Source: ui.Rect{X: 1, W: 2, H: 2},
			Clip: ui.Rect{X: 2, Y: 1, W: 4, H: 2}, Tint: ui.RGBA(255, 255, 255, 128), Filter: ui.FilterBilinear},
		{Kind: ui.CommandQuad, Rect: ui.Rect{W: 2, H: 2}, Clip: ui.Rect{W: 10, H: 10}, Tint: ui.RGBA(0, 0, 255, 255)},
	}}
	list, _ := Freeze(frame, nil)
	mesh, err := Build(list, Mesh{})
	if err != nil || len(mesh.Batches) != 3 {
		t.Fatalf("painter order: %+v, %v", mesh.Batches, err)
	}
	imageBatch := mesh.Batches[1]
	if imageBatch.Filter != ui.FilterBilinear || imageBatch.Texture != (TextureKey{resource.ID(), resource.Version()}) {
		t.Fatalf("incorrect image sampler/identity: %+v", imageBatch)
	}
	a, b, c := mesh.Vertices[imageBatch.First], mesh.Vertices[imageBatch.First+1], mesh.Vertices[imageBatch.First+2]
	if a.X != 2 || a.Y != 1 || b.X != 6 || c.Y != 3 || a.U != .375 || b.U != .625 || a.V != .25 || c.V != .75 {
		t.Fatalf("clipped geometry or source UV: %+v %+v %+v", a, b, c)
	}
	if a.A < .49 || a.A > .51 || a.R != a.A {
		t.Fatalf("image tint is not premultiplied: %+v", a)
	}
}

func TestCacheVersionsEvictionAndRecovery(t *testing.T) {
	cache := NewTextureCache(8) // Two 1x1 textures; old handles await a fence.
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	a, b := TextureKey{ID: 3, Version: 1}, TextureKey{ID: 3, Version: 2}
	cache.Request(a, source, 1)
	upload := cache.DrainUploads()
	if len(upload) != 1 || !cache.Commit(upload[0], 10) {
		t.Fatal("first upload was lost")
	}
	cache.Request(b, source, 2)
	upload = cache.DrainUploads()
	if len(upload) != 1 || !cache.Commit(upload[0], 11) {
		t.Fatal("new version was not uploaded")
	}
	if got := cache.Collect(0); len(got) != 0 {
		t.Fatalf("destroyed in-flight handle: %v", got)
	}
	if got := cache.Collect(1); len(got) != 0 {
		t.Fatalf("unexpected eviction within budget: %v", got)
	}
	c := TextureKey{ID: 4, Version: 1}
	cache.Request(c, source, 3)
	upload = cache.DrainUploads()
	if !cache.Commit(upload[0], 12) {
		t.Fatal("upload rejected")
	}
	if got := cache.Collect(1); len(got) != 1 || got[0] != 10 {
		t.Fatalf("old version not retired after its fence: %v", got)
	}
	cache.DeviceLost()
	if cache.Commit(upload[0], 13) || cache.Generation() != 2 {
		t.Fatal("accepted an upload from the lost device")
	}
	if id, resident := cache.Request(b, source, 4); resident || id != 0 || len(cache.DrainUploads()) != 1 {
		t.Fatal("lost device did not requeue CPU resource")
	}
}

func TestCacheTrimsFormerlyActiveResources(t *testing.T) {
	cache := NewTextureCache(4)
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for id := uint64(1); id <= 2; id++ {
		cache.Request(TextureKey{ID: id, Version: 1}, source, 1)
	}
	for i, upload := range cache.DrainUploads() {
		if !cache.Commit(upload, uint32(i+1)) {
			t.Fatal("failed to upload in-flight resource")
		}
	}
	if cache.PendingRetirements() != 0 {
		t.Fatal("evicted texture from the active frame")
	}
	cache.Request(TextureKey{ID: 2, Version: 1}, source, 2)
	cache.Trim(2)
	if got := cache.Collect(1); len(got) != 1 || got[0] != 1 {
		t.Fatalf("did not release the old frame's unused texture: %v", got)
	}
}

func TestFactoryFallbackAndExplicitSelection(t *testing.T) {
	software := func() (Backend, error) { return &stubBackend{}, nil }
	factory := Factory{Software: software}
	selection, err := factory.Open(Auto)
	if err != nil || selection.Backend.Kind() != Software || selection.Fallback != ErrUnavailable {
		t.Fatalf("auto fallback: %+v, %v", selection, err)
	}
	if _, err := factory.Open(Hardware); err != ErrUnavailable {
		t.Fatalf("hardware preference silently fell back: %v", err)
	}
}

type stubBackend struct{}

func (*stubBackend) Kind() Kind                                         { return Software }
func (*stubBackend) Close() error                                       { return nil }
func (*stubBackend) RenderAtScale(ui.Frame, *image.RGBA, float32) error { return nil }

type failedHardware struct{ closed int }

func (*failedHardware) Kind() Kind { return Hardware }
func (b *failedHardware) Close() error {
	b.closed++
	return nil
}
func (*failedHardware) RenderAtScale(ui.Frame, *image.RGBA, float32) error { return ErrDeviceLost }

func TestFailoverAfterRenderLoss(t *testing.T) {
	hardware := &failedHardware{}
	frame := ui.Frame{Width: 1, Height: 1}
	err := hardware.RenderAtScale(frame, nil, 1)
	fallback, fallbackErr := Failover(hardware, err, func() (Backend, error) { return &stubBackend{}, nil })
	if fallbackErr != nil || fallback.Kind() != Software || hardware.closed != 1 {
		t.Fatalf("failover: %v, %v, closed %d", fallback, fallbackErr, hardware.closed)
	}
	if err = fallback.RenderAtScale(frame, image.NewRGBA(image.Rect(0, 0, 1, 1)), 1); err != nil {
		t.Fatalf("same frame cannot be retried: %v", err)
	}
	if _, err := Failover(&stubBackend{}, ErrDeviceLost, func() (Backend, error) { return &stubBackend{}, nil }); err != ErrDeviceLost {
		t.Fatalf("software failure should propagate: %v", err)
	}
}
