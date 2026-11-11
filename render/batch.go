package render

import (
	"fmt"
	"image"

	ui "github.com/banditmoscow1337/nativeForms"
)

// TextureKey identifies immutable CPU pixels. ID zero is the shared font
// atlas/solid texel; images use ImageResource's ID and version.
type TextureKey struct{ ID, Version uint64 }

type Batch struct {
	Texture TextureKey
	Image   *ui.ImageResource
	Filter  ui.ImageFilter
	First   int
	Count   int
}

type Mesh struct {
	Vertices []ui.Vertex
	Batches  []Batch
	scratch  []ui.Vertex
}

// MaskTexture identifies the dynamic glyph atlas. Zero remains the immutable
// font atlas used by ordinary quads, lines and rounded rectangles.
var MaskTexture = TextureKey{ID: ^uint64(0)}

// Build preserves painter's order. Adjacent draws sharing a texture and
// sampler form one batch; clipping is baked into geometry and image UVs.
func Build(list *DisplayList, mesh Mesh) (Mesh, error) {
	return BuildWithAtlas(list, mesh, nil)
}

// BuildWithAtlas puts repeated masks into one texture and emits one quad per
// mask. When the atlas fills, the original tessellation remains correct.
func BuildWithAtlas(list *DisplayList, mesh Mesh, atlas *GlyphAtlas) (Mesh, error) {
	if list == nil {
		return Mesh{}, fmt.Errorf("render: nil display list")
	}
	return buildCommands(list.commands, mesh, atlas)
}

func buildCommands(commands []ui.DrawCommand, mesh Mesh, atlas *GlyphAtlas) (Mesh, error) {
	mesh.Vertices = mesh.Vertices[:0]
	mesh.Batches = mesh.Batches[:0]
	if minVerts := len(commands) * 6; cap(mesh.Vertices) < minVerts {
		mesh.Vertices = make([]ui.Vertex, 0, minVerts)
	}
	for i := range commands {
		command := commands[i]
		if command.Kind > ui.CommandRoundedRect {
			return Mesh{}, fmt.Errorf("render: unsupported command %d", command.Kind)
		}
		first := len(mesh.Vertices)
		var key TextureKey
		var resource *ui.ImageResource
		filter := ui.FilterNearest
		if command.Kind == ui.CommandImage {
			if command.Image == nil || command.Filter > ui.FilterBilinear {
				return Mesh{}, fmt.Errorf("render: invalid image command")
			}
			resource, filter = command.Image, command.Filter
			key = TextureKey{ID: resource.ID(), Version: resource.Version()}
			mesh.Vertices = appendImage(mesh.Vertices, command)
		} else if command.Kind == ui.CommandMask && atlas != nil {
			if command.Rect.Empty() || command.Clip.Empty() || command.Mask == nil || command.Tint.A <= 0 {
				continue
			}
			if rect, ok := atlas.Place(command.Mask); ok {
				key = MaskTexture
				mesh.Vertices = appendAtlasMask(mesh.Vertices, command, rect, atlas.Size())
			} else {
				mesh.Vertices = ui.AppendTessellatedCommand(command, mesh.Vertices)
			}
		} else {
			mesh.Vertices = ui.AppendTessellatedCommand(command, mesh.Vertices)
		}
		count := len(mesh.Vertices) - first
		if count == 0 {
			continue
		}
		if n := len(mesh.Batches); n > 0 && mesh.Batches[n-1].Texture == key && mesh.Batches[n-1].Filter == filter {
			mesh.Batches[n-1].Count += count
		} else {
			mesh.Batches = append(mesh.Batches, Batch{Texture: key, Image: resource,
				Filter: filter, First: first, Count: count})
		}
	}
	return mesh, nil
}

// BuildFrame is the synchronous path for a native renderer. The returned mesh
// owns its vertices, so the Manager may reuse its command storage afterwards.
func BuildFrame(frame ui.Frame, mesh Mesh, atlas *GlyphAtlas) (Mesh, error) {
	if frame.Width <= 0 || frame.Height <= 0 {
		return Mesh{}, fmt.Errorf("render: invalid frame dimensions")
	}
	return buildCommands(frame.Commands, mesh, atlas)
}

func appendAtlasMask(dst []ui.Vertex, c ui.DrawCommand, region image.Rectangle, size int) []ui.Vertex {
	if c.Rect.Empty() || c.Clip.Empty() || c.Tint.A <= 0 {
		return dst
	}
	x0, y0 := max(c.Rect.X, c.Clip.X), max(c.Rect.Y, c.Clip.Y)
	x1, y1 := min(c.Rect.X+c.Rect.W, c.Clip.X+c.Clip.W), min(c.Rect.Y+c.Rect.H, c.Clip.Y+c.Clip.H)
	if x1 <= x0 || y1 <= y0 {
		return dst
	}
	invSize := 1.0 / float32(size)
	uBase := float32(region.Min.X) * invSize
	vBase := float32(region.Min.Y) * invSize
	uScale := (float32(region.Dx()) / c.Rect.W) * invSize
	vScale := (float32(region.Dy()) / c.Rect.H) * invSize
	u0 := uBase + (x0-c.Rect.X)*uScale
	u1 := uBase + (x1-c.Rect.X)*uScale
	v0 := vBase + (y0-c.Rect.Y)*vScale
	v1 := vBase + (y1-c.Rect.Y)*vScale
	tr, tg, tb, ta := c.Tint.R, c.Tint.G, c.Tint.B, c.Tint.A
	return append(dst,
		ui.Vertex{X: x0, Y: y0, U: u0, V: v0, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x1, Y: y0, U: u1, V: v0, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x0, Y: y1, U: u0, V: v1, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x0, Y: y1, U: u0, V: v1, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x1, Y: y0, U: u1, V: v0, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x1, Y: y1, U: u1, V: v1, R: tr, G: tg, B: tb, A: ta},
	)
}

func appendImage(vertices []ui.Vertex, c ui.DrawCommand) []ui.Vertex {
	if c.Rect.Empty() || c.Clip.Empty() || c.Tint.A <= 0 {
		return vertices
	}
	bounds := c.Image.Bounds()
	if bounds.Empty() {
		return vertices
	}
	source := c.Source
	if source == (ui.Rect{}) {
		source = ui.Rect{W: float32(bounds.Dx()), H: float32(bounds.Dy())}
	}
	if source.Empty() {
		return vertices
	}
	x0, y0 := max(c.Rect.X, c.Clip.X), max(c.Rect.Y, c.Clip.Y)
	x1, y1 := min(c.Rect.X+c.Rect.W, c.Clip.X+c.Clip.W), min(c.Rect.Y+c.Rect.H, c.Clip.Y+c.Clip.H)
	if x1 <= x0 || y1 <= y0 {
		return vertices
	}
	invW := 1.0 / float32(bounds.Dx())
	invH := 1.0 / float32(bounds.Dy())
	u0 := (source.X + (x0-c.Rect.X)*source.W/c.Rect.W) * invW
	u1 := (source.X + (x1-c.Rect.X)*source.W/c.Rect.W) * invW
	v0 := (source.Y + (y0-c.Rect.Y)*source.H/c.Rect.H) * invH
	v1 := (source.Y + (y1-c.Rect.Y)*source.H/c.Rect.H) * invH
	tr, tg, tb, ta := c.Tint.R*c.Tint.A, c.Tint.G*c.Tint.A, c.Tint.B*c.Tint.A, c.Tint.A
	return append(vertices,
		ui.Vertex{X: x0, Y: y0, U: u0, V: v0, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x1, Y: y0, U: u1, V: v0, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x0, Y: y1, U: u0, V: v1, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x0, Y: y1, U: u0, V: v1, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x1, Y: y0, U: u1, V: v0, R: tr, G: tg, B: tb, A: ta},
		ui.Vertex{X: x1, Y: y1, U: u1, V: v1, R: tr, G: tg, B: tb, A: ta},
	)
}
