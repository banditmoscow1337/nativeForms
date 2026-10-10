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
| windows | amd64 | Win32 | `windows-2025` | Pending owner run | Pending owner run |
| windows | arm64 | Win32 | `windows-11-arm` | Pending owner run | Pending owner run |
| linux | amd64 | X11 and Wayland | `ubuntu-24.04` | Pending owner run | Pending owner run on both sessions |
| linux | arm64 | X11 and Wayland | `ubuntu-24.04-arm` | Pending owner run | Pending owner run on both sessions |
| darwin | amd64 | AppKit | `macos-15-intel` | Pending owner run | Pending owner run |
| darwin | arm64 | AppKit | `macos-15` | Pending owner run | Pending owner run |

The runner labels are current examples for native builds, not minimum OS
versions promised to application users. No OS-version floor has been validated.
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
a short note and any relevant log. Leave the matrix above pending until these
results exist.

