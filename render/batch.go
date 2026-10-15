package render

import (
	"fmt"

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

// Build preserves painter's order. Adjacent draws sharing a texture and
// sampler form one batch; clipping is baked into geometry and image UVs.
func Build(list *DisplayList, mesh Mesh) (Mesh, error) {
	if list == nil {
		return Mesh{}, fmt.Errorf("render: nil display list")
	}
	mesh.Vertices = mesh.Vertices[:0]
	mesh.Batches = mesh.Batches[:0]
	for _, command := range list.commands {
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
		} else {
			mesh.scratch = ui.Tessellate([]ui.DrawCommand{command}, mesh.scratch)
			mesh.Vertices = append(mesh.Vertices, mesh.scratch...)
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
	u := func(x float32) float32 {
		return (source.X + (x-c.Rect.X)*source.W/c.Rect.W) / float32(bounds.Dx())
	}
	v := func(y float32) float32 {
		return (source.Y + (y-c.Rect.Y)*source.H/c.Rect.H) / float32(bounds.Dy())
	}
	point := func(x, y float32) ui.Vertex {
		return ui.Vertex{X: x, Y: y, U: u(x), V: v(y), R: c.Tint.R * c.Tint.A,
			G: c.Tint.G * c.Tint.A, B: c.Tint.B * c.Tint.A, A: c.Tint.A}
	}
	return append(vertices, point(x0, y0), point(x1, y0), point(x0, y1),
		point(x0, y1), point(x1, y0), point(x1, y1))
}
