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

The implementation covers the remaining native-adapter work. Platform execution
and acceptance checks are left to the project owner; no builds or tests were run
here. Only the standard library and `purego` are used by these adapters. The
optional Vulkan package continues to use the author's `miniVK`.

| Backend | Frame/input | Clipboard | IME | Multiple windows | Incoming file drops | File dialogs |
| --- | --- | --- | --- | --- | --- | --- |
| Win32 | Software, Win32 input | UTF-16 | IMM32 | Independent UI threads | Shell HDROP | Open/Save panels |
| AppKit | Software, Core Text | NSPasteboard | NSTextInputClient | One shared AppKit loop | File URLs and path lists | NSOpenPanel/NSSavePanel |
| Local X11 | PutImage, keysyms, compose | UTF-8, including INCR | IBus when available | Independent connections | Xdnd 3–5, copy | Desktop portal when available |
| Native Wayland | xdg-shell, wl_shm, XKB, compose | wl_data_device | text-input-v3 when advertised | Independent connections | Local file URI lists | Desktop portal when available |

### Capability discovery and UI ownership

Use the **window instance's** `Capabilities()` from `OnReady` or later for
session-dependent features. Package-level helpers describe the baseline backend;
Linux IME and file-dialog flags require runtime discovery. `FileDialog` depends
on a working `org.freedesktop.portal.FileChooser` service. X11 IME depends on an
IBus input context; Wayland IME depends on the compositor advertising
`zwp_text_input_manager_v3`. Wayland clipboard/drop needs a seat and data device.
There is no Xwayland fallback and no external helper executable.

All tree operations, clipboard access and file dialogs run on the owning UI
goroutine. Use `Manager.Post` for work from another goroutine. `Close` is a
thread-safe exit request. Use a distinct manager for each window. Normal native
loops wait at most 50 ms before checking for posted work. Protocol requests have
bounded waits; an open file picker waits for the user and closes its portal
request when its parent window closes.

`OpenFile()` and `SaveFile(defaultName)` return an empty path with no error on
cancellation. These methods choose paths; they do not read, write or overwrite
files. `OnFilesDropped` receives local paths on the UI goroutine. Drag sources,
non-file payloads and move/delete semantics are outside this file-drop API.

### X11

CLIPBOARD supports TARGETS, UTF8_STRING, TEXT and Latin-1 STRING. Both incoming
and outgoing large values use ICCCM INCR, with bounded chunks, a 16 MiB transfer
limit and timeout cleanup. GetProperty replies keep partial packets across
wakeups and are routed by sequence during nested event dispatch. A vanished
selection requestor does not terminate the owner's event loop. The owner must
keep running for other clients to request its selection.

Xdnd advertises version 5 and accepts a copy of `text/uri-list`. It negotiates
types/actions, receives XdndSelection through the same selection machinery,
acknowledges completion and decodes local paths. Nonlocal file hosts are rejected.

Input uses the server's keysyms and group, updates keyboard mapping on
MappingNotify, handles Shift/CapsLock/NumLock and Unicode/legacy keysym mapping.
Locale compose data is read from XCompose files, including user overrides and
bounded includes. IBus is accessed directly over authenticated Unix D-Bus,
using `IBUS_ADDRESS`, the display-specific IBus address file, or the session bus. IBus supplies
preedit, commit, surrounding-text deletion and candidate placement. This adapter
uses **IBus rather than implementing the separate XIM transport**. Without IBus,
ordinary keysyms and compose remain available; `IME` is false.

### Wayland

The compositor owns keyboard actions and modifier state. The first-party reader
consumes its resolved XKB keymap: keycodes, aliases, symbols, per-group types,
modifier-to-level maps, virtual modifiers, multiple symbols per level and group
wrap/clamp/redirect. It uses effective modifier/group events and compositor
repeat settings; repeat stops on release or focus loss. Compose uses the same
XCompose reader as X11. The built-in US map is the fallback if no usable keymap
was supplied. Source XKB `include` files are not evaluated by this client.

Text-input-v3 binds version 1. Enable/disable follows editor and surface focus;
preedit/commit/delete events are applied on `done`, with serial tracking,
bounded UTF-8 surrounding text, password hints and cursor rectangles. Clipboard
and file drop still use the data-device protocol and file descriptors with a
16 MiB limit. Linux file dialogs use the desktop portal via the first-party
D-Bus codec. X11 provides a parent XID; Wayland currently passes an empty parent
identifier, so its picker can be unparented on some desktops.

### AppKit

Lock the main goroutine to the startup OS thread in `main`'s `init`:

```go
func init() { runtime.LockOSThread() }

func main() {
    // Each window has its own manager and component tree.
    if err := darwin.Run(firstWindow, secondWindow); err != nil {
        log.Fatal(err)
    }
}
```

`Window.Run()` is shorthand for `darwin.Run(window)`. Call `Window.Open()` from
an existing window's UI callback to add another window. The shared loop routes
events by NSWindow and closes only the selected window. It exits after the last
window closes. Calling `Run` again concurrently returns an error. NSWindow has
`releasedWhenClosed = false`; its owning reference, view and bitmap are released
once, after dispatch returns. See `examples/multiwindow_darwin` for two initial
windows and a button that opens more.

A registered NSImageView subclass implements NSTextInputClient, including
marked/selected ranges, UTF-16 conversion, substring queries, candidate
coordinates, command selectors and text insertion. Key events pass through
`interpretKeyEvents` once. Marked text is a virtual replacement of the selected
text, with its own selection/caret, and does not enter undo history until commit.

`Run` installs Core Text automatically. If the app measures text before running,
call `darwin.EnableCoreText()` first. Core Text shapes whole lines, performs system
font fallback, supplies caret offsets and rasterizes immutable alpha masks for
the software renderer. Its cache is bounded. This native path uses system fonts;
`SetFontData` configures the separate portable atlas. `ui.SetTextShaper(nil)`
restores that portable path; invalidate existing layouts after changing shapers.
The shared mask command has a Vulkan fallback using alpha spans, which can reach
Vulkan's existing 64K-vertex limit sooner than atlas text. Masks are monochrome,
including glyphs from color fonts.

The portable font repertoire, full Unicode grapheme tables, rich text, advanced
bidi selection, accessibility and touch remain separate typography/product
milestones. Implementing native IME does not expand the portable atlas repertoire.

### Owner's acceptance checklist

The editor examples now expose Open/Save buttons and display dropped file paths.
Run the existing examples on their respective systems and check:

1. Close by the title-bar button, by `Close`, and after using an IME or file picker.
2. On macOS, run `go run ./examples/multiwindow_darwin`; close one window, continue
   editing the other, open another, then close the last window.
3. Type Russian/Latin, dead keys, compose sequences, AltGr characters, held keys,
   and IME preedit/commit/cancel; switch focus during composition. Check undo,
   selection replacement and candidate position, including non-BMP characters.
4. Copy/paste a short value, an empty value and a value larger than one X11 request
   between nativeForms and another application. Close the peer during transfer.
5. Drop multiple filenames containing spaces/non-ASCII characters; cancel a drop.
6. Open/Save and cancel native pickers on X11 and Wayland. Without a portal or IME
   service, verify the corresponding capability is false and basic input works.
7. Check macOS Core Text alignment, ligatures, fallback glyphs, caret position and
   clipping in the same editor/offscreen geometry.

Code was formatted and statically inspected. These runtime checks have **not**
been performed by the agent, as requested.
