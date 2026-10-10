# Desktop baseline: supported targets and owner verification

The desktop target is limited to Windows, Linux and macOS on **amd64** and
**arm64**. These six combinations are the release matrix. Builds on other
operating systems or architectures are outside this scope even when the portable
core happens to compile there. The game continues to use the optional Vulkan
package; the standalone examples use the software renderer.

This baseline starts at commit `23a5af5d48f1985e43774cc535e1ca1ce015e2aa`.
That commit is the reference for comparing the existing game's integration and
the UI examples as the toolkit changes. It is not a claim that the commit has
passed builds or runtime acceptance. The current `go.mod` specifies Go
**1.27.0** as the minimum toolchain version. Do not lower it without inspecting
the code and dependencies and repeating the matrix.

## Support matrix

| GOOS | GOARCH | Desktop adapter | GitHub runner | Build | Native runtime |
| --- | --- | --- | --- | --- | --- |
| windows | amd64 | Win32 | `windows-2025` | Passed, baseline PR #8 | Pending owner run |
| windows | arm64 | Win32 | `windows-11-arm` | Passed, baseline PR #8 | Pending owner run |
| linux | amd64 | X11 and Wayland | `ubuntu-24.04` | Passed, baseline PR #8 | Pending owner run on both sessions |
| linux | arm64 | X11 and Wayland | `ubuntu-24.04-arm` | Passed, baseline PR #8 | Pending owner run on both sessions |
| darwin | amd64 | AppKit | `macos-15-intel` | Passed, baseline PR #8 | Pending owner run |
| darwin | arm64 | AppKit | `macos-15` | Passed, baseline PR #8 | Pending owner run |

The runner labels are current examples for native builds, not minimum OS
versions promised to application users. No OS-version floor has been validated.
The owner reported all six baseline checks passing before merging PR #8;
these results do not validate native runtime interaction or this scheduler PR.
The Windows adapter documents Windows 10 with `GetDpiForWindow`; macOS and Linux
version floors should be recorded only after runtime checks on the intended
systems. On Linux, actual capabilities also depend on the display server,
compositor, IBus/text-input-v3 and desktop portal. Query the window's
`Capabilities()` after startup.

## Owner-run CI

The workflow in [desktop-baseline.yml](../.github/workflows/desktop-baseline.yml)
builds and runs headless package tests on native runners for all six targets.
Its Linux/amd64 job also checks that the core and software import graph does
not include Vulkan or miniVK.
It is deliberately not triggered by pushes or by creating a draft PR. When
reviewing a draft PR containing this workflow, choose **Ready for review** to
start the six jobs. `workflow_dispatch` becomes available in the Actions UI
once the workflow file is on the default branch. The workflow checks package
compilation, including examples and the optional Vulkan package; it cannot
exercise a native desktop session or an external game's renderer.

Please record each result in the matrix above after your run. A failed job is
evidence for that target, not a reason to mark the other targets as working.
Keep compiler output and the exact commit with the report.

## Scheduler and idle behavior

`Manager.NextFrameDeadline()` returns an absolute deadline and a separate
presence flag. An idle UI has no deadline; a deadline in the past is due and
requests a frame through `NeedsFrame()`. Window loops must check `NeedsFrame()`
after their wait returns. On Wayland a frame also needs an available buffer;
when both buffers are busy the loop waits for a release instead of retrying
the expired deadline. `NextFrameAfter()` remains available for older hosts but
its zero value cannot distinguish no timer from an elapsed deadline.

Notifications and caret blinking use Go's monotonic wall time. `BeginFrame`'s
`deltaSeconds` argument remains for API compatibility and does not consume a
new notification's lifetime. Suspension saves remaining notification time and
starts it again on restore. A hidden manager does not schedule timer frames;
expired notifications are removed when it becomes visible again. A game can
continue to call `BeginFrame` on its own schedule.

The headless `scheduler_test.go` covers idle, caret, delayed first notification
frame, staggered expirations, refresh, suspension, hidden UI and queued work.
For native acceptance, leave a static window idle for 10 seconds and observe
no new UI frames; then focus a text field to observe caret blinking, and show
a short notification after a long idle period to observe its full lifetime.
Repeat on Windows, macOS, X11 and Wayland; also hide/restore the window and
check the Wayland path when both buffers are busy.

## Input and icon acceptance

The headless input regression tests cover hover delivery and bubbling without
pointer capture, dragging out of a menu, disabled menu items, keyboard selection
in ComboBox, modal Tab focus and Escape during IME composition. ComboBox and
VirtualTree chevrons are now line commands shared by software and Vulkan; no
arrow glyph is required from the portable font atlas.

For native acceptance, move the pointer between menu rows without pressing a
button, then drag outside the window and release. In a dialog with a focused
text field, start a real IME preedit: the first Escape should cancel the
composition and leave the dialog open; the next Escape should close it. Check
Tab and Shift+Tab across enabled controls, keyboard opening and selection in a
ComboBox, and chevrons with a font that has no arrow glyphs. Repeat on Windows,
macOS, X11 and Wayland; headless tests cannot validate OS IME event ordering.

## Local checks to run on each target

Use Go 1.27.0 or newer. In a checkout at the commit you want to accept, run:

```sh
go version
go env GOOS GOARCH
go build ./...
go test ./...
go run ./examples/offscreen
go run ./examples/text_ru_en
```

Inspect `frame.png`, `text-ru-en.png` and `text-ru-en.json`. These examples
write files to the current directory. Use a font with Cyrillic coverage for
text inspection. Then start the native editor example on the same machine:

| OS | Command | Additional run |
| --- | --- | --- |
| Windows | `go run ./examples/editor_windows` | `go run ./examples/windows` |
| Linux on X11 | `go run ./examples/editor_linux` with `WAYLAND_DISPLAY` unset and `DISPLAY` set | X11 session |
| Linux on Wayland | `go run ./examples/editor_linux` with `WAYLAND_DISPLAY` set | Native Wayland session |
| macOS | `go run ./examples/editor_darwin` | `go run ./examples/multiwindow_darwin` |

On each native run, check window creation and close, typing in English and
Russian, selection and undo, clipboard to and from another application,
resize, focus change, file drop and native Open/Save/cancel. Check a short idle
period without input, then caret blink and notification expiration. Move an
open window between monitors with different scale factors where available.
On Linux, also check the absence of the IME or portal service and capture the
corresponding `Capabilities()` values. The extended input/protocol checklist
is in [stages 12–13](stages-12-13.md).

For the game, build and run the actual host using its normal Vulkan renderer,
exercise a UI overlay with text, scrolling and clipping, then close and reopen
it. `go build ./vulkan` checks the package but does not verify that host's
shaders, framebuffer contract or GPU resource lifetime.

Record for every run: commit, GOOS/GOARCH, `go version`, OS version, display
server or compositor, device/DPI and font choice. Mark build, example,
desktop interaction and Vulkan host separately as pass, fail or not run, with
a short note and any relevant log. Leave native runtime entries pending until
those results exist.

