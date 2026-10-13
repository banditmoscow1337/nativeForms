# ADR 0002: Desktop application, windows and tray lifecycle

Status: proposed for v1; stage 0 does not add a tray implementation.

Separate application lifetime from any one window. `App.Run` owns the OS
event loop and the UI thread; zero visible windows is valid while a tray item
is active. A window's close request must have an explicit policy (hide to
tray, close window, quit application) and a cancelable callback. Quitting
removes tray resources, closes windows and releases renderers once. Reopening
from a tray menu must create/show a window on the UI thread; off-thread work
uses `Manager.Post`. Native callbacks must not synchronously wait on the
thread that is servicing them.

Public API sketch to validate in a later stage: `App`, `Window`, `Tray`,
`MenuItem` with stable IDs, enabled/checked state, separator, nested menu,
icon and activation callback. Keep OS handles private. Distinguish icon click
from context-menu action and ensure callbacks are serialized on the UI
thread. The tray should remain functional even when no `Manager` needs a
frame. Existing `Run(manager, options)` wrappers can delegate to `App` for
source compatibility where feasible.

Windows uses `Shell_NotifyIcon` and a menu attached to the UI message loop;
macOS uses `NSStatusItem`/`NSMenu` on the main thread. Linux should support
StatusNotifierItem over D-Bus where a host exists and report capability
unavailability explicitly; X11-only legacy tray behavior is not assumed on
Wayland. Never silently claim tray support when no host is present. Test
double activation, hide/show, menu mutation, multiple windows, app quit,
explorer/host restart, and tray removal. Record platform-specific limits in
the eventual public API documentation.
