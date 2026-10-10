# nativeForms

Go desktop UI with a software renderer and native window adapters for Windows,
macOS, X11 and Wayland. The optional `vulkan` package is separate from the
software renderer.

## Images and icons

`Image` displays an immutable `ImageResource`. Build one from an `image.Image`
with `NewImageResource`, or decode PNG, JPEG and GIF data from an `io.Reader`
with `DecodeImageResource`. Reuse a resource in as many widgets as needed.

```go
import (
    "os"
    ui "github.com/banditmoscow1337/nativeForms"
)

func imageExample(openFile func()) (ui.Component, error) {
    file, err := os.Open("assets/open.png")
    if err != nil { return nil, err }
    defer file.Close()
    icon, err := ui.DecodeImageResource(file)
    if err != nil { return nil, err }

    preview := ui.NewImage(icon)
    preview.SetPreferredSize(ui.Vec2{X: 120, Y: 80})
    preview.SetFit(ui.ImageContain) // also ImageCover or ImageStretch
    button := ui.NewButton("Open", openFile)
    button.SetIcon(icon)
    menu := ui.NewMenu([]ui.MenuItem{
        {ID: "open", Label: "Open", Icon: icon},
    }, nil)
    panel := ui.NewStack(ui.Vertical, 8)
    panel.Add(preview, button, menu)
    return panel, nil
}
```

`Image` uses bilinear filtering by default; call `SetFilter(FilterNearest)` for pixel art.
`SetTint` changes color and opacity. `SetImage`, `SetFit`, `SetFilter` and
`SetTint` request a new frame when attached to a `Manager`. Use `Replace` on
the old resource to create a new immutable version, then pass it to `SetImage`
or `SetIcon`; existing frames keep their original snapshot.

`Button.Icon`, `Checkbox.Icon`, `Toggle.Icon`, `MenuItem.Icon`,
`RadioOption.Icon` (radio groups and combo boxes), and `Tab.Icon` accept an
`*ImageResource`. Icons keep their aspect ratio in a 20 logical pixel slot,
with bilinear filtering. `SetIcon` on the controls updates layout automatically.
An icon-only button should have an accessibility name:
`button.SetAccessibility(nativeforms.AccessibilityInfo{Name: "Open"})`.

Virtual models can supply icons without changing their existing interfaces:
implement `ListIconModel.Icon(index)`, `TreeIconModel.Icon(id)`, or
`TableIconModel.CellIcon(row, column)`. Return `nil` for rows without an icon;
the label column remains aligned. Only visible rows are queried while painting.
The software renderer supports images at the display scale on all desktop
adapters. The optional Vulkan backend currently rejects image commands.

## Desktop integration

| Backend | Clipboard | Text input | File chooser | File drop |
| --- | --- | --- | --- | --- |
| Windows | Unicode text | Win32 IME | Native open/save dialog | Explorer file drop |
| macOS | Pasteboard | AppKit text input | Native open/save panel | Finder file drop |
| X11 | UTF-8 selection | IBus when available; key compose fallback | Desktop portal when available | Xdnd URI list |
| Wayland | Data device when available | text-input-v3 when available; key compose fallback | Desktop portal when available | Data device URI list |

Each adapter has `Capabilities()`. Query the running window on Linux: IME,
clipboard and portal support depend on the compositor, seat, session bus and
installed services. A missing file portal returns an error wrapping
`platform.ErrUnsupported`. Wayland clipboard writes require a recent seat input
serial; calls before any input return an error. File chooser cancellation
returns an empty path with no error.

## Display scale

`Options.Width` and `Options.Height` describe the initial **logical client
area**. UI layout, pointer events and text caret rectangles stay in logical
coordinates. Desktop adapters render to a separate physical pixel buffer and
update `Manager.DisplayScale()` as the window's display scale changes. Glyphs
drawn through `FontContext` are rasterized at the physical scale. Applications
can use `Manager.SetFontContext(nativeforms.NewSystemFontContext())` or supply
their own font data.

- Windows uses per-monitor DPI when available, applies the suggested window
  rectangle on `WM_DPICHANGED`, and sizes the initial client area for its DPI.
- macOS uses the view's backing size and the window's backing scale; moving a
  window to a different display recreates the pixel buffer.
- Wayland uses the compositor's fractional scale and `wp_viewporter` when both
  protocols are offered. Otherwise it uses `wl_output.scale` with
  `wl_surface.set_buffer_scale` (or scale 1 on older compositors). Output changes
  retire old buffers safely.
- X11 reads `Xft.dpi` from the root `RESOURCE_MANAGER` at startup. Set
  `X11Options.Scale` explicitly to override it, for example for a display that
  does not publish X resources.

For headless rendering, pass logical dimensions to `Manager.BeginFrame`, set
`Manager.SetDisplayScale(scale)`, allocate a physical RGBA buffer, then use
`software.Renderer.RenderAtScale(frame, buffer, scale)`. A physical width that
is not divisible by the scale is clipped at the buffer edge.

Run `go test ./...` for host tests. The desktop baseline GitHub Actions workflow
builds amd64 and arm64 adapters for Windows, Linux and macOS and runs their
platform tests.
