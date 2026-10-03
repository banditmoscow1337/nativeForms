# Stages 12–13: editor widgets and desktop adapters

## Stage 12: component tree and overlays

`Manager.OpenPopup(content, bounds, modal)` paints content above the ordinary
tree. Each popup has a unique `PopupID`; `ClosePopup` restores prior focus.
Modal popups dim underlying content and stop pointer hits and Tab navigation
from escaping their subtree. Escape closes the top interactive popup. Bounds
with zero width or height use the measured content size and are clamped to the
viewport. `OpenDialog` centers a modal component in the current viewport.
Use `NewDialog` to build its body, title, and actions.

`Checkbox`, `RadioGroup`, `ComboBox`, `Menu`, and `Tabs` expose stable string
IDs for choices. Menus can be opened with `OpenMenu` or `OpenContextMenu`.
`State.SetTooltip` shows a passive overlay while the target is hovered;
`Manager.ShowTooltip` is available for explicit placement. `NewSplitPane`
from the previous stage supplies a draggable divider. Theme has `Selection`,
`Tooltip`, `Scrim`, and `Header` colors in addition to the original tokens.
AddShortcut/RemoveShortcut register actions with exact key and modifier
matching; focused editors get the key before an application shortcut.

`VirtualList`, `VirtualTable`, and `VirtualTree` use application-owned models.
List and table painting, cell lookup, and pointer hit testing touch only rows
and columns intersecting the viewport. Tree flattening visits expanded nodes
when arranged, then paints viewport rows. A model's `ID` must be unique and
stable through sorting or insertion; selection stores that ID. After changing
model data, call `Manager.InvalidateLayout`. `examples/editor` builds one
scene hierarchy, object table, property pane, menus, tabs, and modal dialog;
the three desktop entry points import the same application tree.

## Stage 13: native adapters

`platform.Capabilities` exposes what a backend actually implements. A false
value means callers should disable that feature. `platform.ErrUnsupported`
can be checked with `errors.Is` for requests that cannot be served. The
implementation uses the Go standard library, `purego` for AppKit/Win32, and
the author's own optional `miniVK` for Vulkan; no new third-party dependency
was added.

| Backend | Frame | Pointer | Keyboard and basic text | Clipboard | IME | Multiwindow | Drag and drop | File dialogs |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Win32 | Yes | Yes | Yes | Yes | Yes | Yes | File paths from shell | Yes |
| AppKit via purego | Yes | Yes | Yes | Yes | No | No | No | Yes |
| Local X11 wire protocol | Yes | Yes | Core keysyms | No | No | Yes | No | No |
| Wayland xdg-shell + shm | Yes | Yes | US evdev fallback | No | No | Yes | No | No |

The AppKit adapter uses `NSApplication`, `NSWindow`, `NSBitmapImageRep`,
`NSImageView`, `NSEvent`, and `NSPasteboard`. It currently draws text with the
framework's shared TrueType atlas, not Core Text. The X11 adapter opens a
Unix display socket, negotiates MIT-MAGIC-COOKIE-1, checks the default visual,
creates a window, and sends rows as bounded `PutImage` requests. It handles
pointer motion and buttons, wheel, resize, core keyboard mapping, and basic
text entry. X11 input does not yet handle locale compose, XIM, or clipboard
selection. `ProbeWayland` performs `wl_display.get_registry` and sync on a
Unix socket. `WaylandWindow.Run` binds the compositor, shared memory, seat,
and xdg-shell, then presents software frames with `wl_shm` buffers. It handles
configure/close, pointer movement and buttons, scrolling, and basic evdev
keys. Keyboard text currently assumes a US layout regardless of compositor
keymap; Cyrillic input, compose, repeat, and IME require further work.
`Close()` requests exit and the read loop observes it within 50 ms. It does
not open an Xwayland window.

The AppKit window keeps ownership after the title-bar close action and releases
its reference only after the event loop has returned. Win32 routes each HWND
to its own `Window`; each `Run` owns a locked UI thread and its own manager.
Linux windows use independent display connections. The Windows shell file drop
handler passes UTF-16 paths to `Options.OnFilesDropped` and releases the HDROP.
Win32 and AppKit provide `OpenFile` and `SaveFile`; cancellation returns an
empty path and no error. Both must be called on the adapter UI goroutine.
On Linux these methods return `platform.ErrUnsupported`.

Stage 13 still has gaps: Linux clipboard, keymap and IME, macOS IME and
multiwindow, Core Text shaping, file drops on macOS/Linux, and native Linux
file dialogs. Keep those feature flags false in production UI until the
respective adapter implements them. The shared editor scenario uses the
features marked Yes above, subject to platform verification.

The owner requested no local builds or tests. Code was formatted and inspected,
but no desktop backend was exercised here.
