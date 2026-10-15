// Package render defines the frame, resource and batching contract shared by
// the software renderer and the engine's Vulkan adapter.
package render

import (
	"bytes"
	"fmt"
	"image"
	"math"

	ui "github.com/banditmoscow1337/nativeForms"
)

// DisplayList owns its command stream and glyph masks. ImageResource already
// owns immutable pixels and versions, so snapshots retain those by reference.
// A list may outlive the Manager's next BeginFrame or an in-flight GPU frame.
type DisplayList struct {
	width, height int
	commands      []ui.DrawCommand
	damage        image.Rectangle
}

func (list *DisplayList) Size() image.Point {
	if list == nil {
		return image.Point{}
	}
	return image.Pt(list.width, list.height)
}

// Damage is a conservative pixel region changed since previous. The first
// snapshot and a resize invalidate the entire viewport.
func (list *DisplayList) Damage() image.Rectangle {
	if list == nil {
		return image.Rectangle{}
	}
	return list.damage
}

// Frame returns a detached copy; callers may edit it without changing a list.
func (list *DisplayList) Frame() ui.Frame {
	if list == nil {
		return ui.Frame{}
	}
	return ui.Frame{Width: list.width, Height: list.height, Commands: cloneCommands(list.commands)}
}

func cloneCommands(commands []ui.DrawCommand) []ui.DrawCommand {
	result := make([]ui.DrawCommand, len(commands))
	masks := make(map[*image.Alpha]*image.Alpha)
	for i, command := range commands {
		if command.Mask != nil {
			mask := masks[command.Mask]
			if mask == nil {
				copyMask := *command.Mask
				copyMask.Pix = append([]byte(nil), command.Mask.Pix...)
				mask = &copyMask
				masks[command.Mask] = mask
			}
			command.Mask = mask
		}
		result[i] = command
	}
	return result
}

func Freeze(frame ui.Frame, previous *DisplayList) (*DisplayList, error) {
	if frame.Width <= 0 || frame.Height <= 0 {
		return nil, fmt.Errorf("render: invalid frame dimensions %d x %d", frame.Width, frame.Height)
	}
	for _, command := range frame.Commands {
		if command.Kind > ui.CommandRoundedRect {
			return nil, fmt.Errorf("render: unsupported command %d", command.Kind)
		}
		if command.Kind == ui.CommandImage && command.Filter > ui.FilterBilinear {
			return nil, fmt.Errorf("render: unsupported image filter %d", command.Filter)
		}
	}
	list := &DisplayList{width: frame.Width, height: frame.Height, commands: cloneCommands(frame.Commands)}
	viewport := image.Rect(0, 0, frame.Width, frame.Height)
	if previous == nil || previous.width != frame.Width || previous.height != frame.Height {
		list.damage = viewport
		return list, nil
	}
	for i := 0; i < max(len(previous.commands), len(list.commands)); i++ {
		if i < len(previous.commands) && i < len(list.commands) && sameCommand(previous.commands[i], list.commands[i]) {
			continue
		}
		if i < len(previous.commands) {
			list.damage = list.damage.Union(commandBounds(previous.commands[i], viewport))
		}
		if i < len(list.commands) {
			list.damage = list.damage.Union(commandBounds(list.commands[i], viewport))
		}
	}
	return list, nil
}

func sameCommand(a, b ui.DrawCommand) bool {
	if a.Kind != b.Kind || a.Rect != b.Rect || a.UV != b.UV || a.Clip != b.Clip ||
		a.Tint != b.Tint || a.From != b.From || a.To != b.To || a.Width != b.Width ||
		a.Source != b.Source || a.Filter != b.Filter || a.Radius != b.Radius {
		return false
	}
	if a.Image != b.Image {
		if a.Image == nil || b.Image == nil || a.Image.ID() != b.Image.ID() || a.Image.Version() != b.Image.Version() {
			return false
		}
	}
	if a.Mask == nil || b.Mask == nil {
		return a.Mask == nil && b.Mask == nil
	}
	return a.Mask.Rect == b.Mask.Rect && a.Mask.Stride == b.Mask.Stride && bytes.Equal(a.Mask.Pix, b.Mask.Pix)
}

func commandBounds(c ui.DrawCommand, viewport image.Rectangle) image.Rectangle {
	r := c.Rect
	if c.Kind == ui.CommandLine {
		half := c.Width * .5
		r = ui.Rect{X: min(c.From.X, c.To.X) - half, Y: min(c.From.Y, c.To.Y) - half,
			W: float32(math.Abs(float64(c.From.X-c.To.X))) + c.Width,
			H: float32(math.Abs(float64(c.From.Y-c.To.Y))) + c.Width}
	}
	x0, y0 := max(r.X, c.Clip.X), max(r.Y, c.Clip.Y)
	x1, y1 := min(r.X+r.W, c.Clip.X+c.Clip.W), min(r.Y+r.H, c.Clip.Y+c.Clip.H)
	if x1 <= x0 || y1 <= y0 {
		return image.Rectangle{}
	}
	return image.Rect(int(math.Floor(float64(x0))), int(math.Floor(float64(y0))),
		int(math.Ceil(float64(x1))), int(math.Ceil(float64(y1)))).Intersect(viewport)
}
