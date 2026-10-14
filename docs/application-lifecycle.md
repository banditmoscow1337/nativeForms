# Desktop application lifetime

`desktop.App` owns one native UI loop, including when all windows are hidden
or closed. Each window has a separate `Manager`. Register windows before
`Run`, or register them inside `App.Post` on the running UI goroutine. `Run`
must execute on the main OS thread on macOS; the desktop adapter locks that
thread during package initialization. Do not block a native callback waiting
for a callback posted to that same loop.

```go
app := desktop.NewApp()
manager := nativeforms.New()
manager.SetRoot(nativeforms.NewLabel("Ready"))
_, err := app.NewWindow(manager, desktop.Options{
    Title: "Example",
    OnCloseRequest: func(w desktop.Window) platform.CloseAction {
        return platform.HideWindow
    },
})
if err != nil { return err }

// A tray menu in the next stage can call Show on the window or app.Quit().
return app.Run()
```

`OnCloseRequest` handles the OS close button and returns `CloseWindow`,
`HideWindow`, `QuitApp`, or `CancelClose`. `Window.Close` explicitly destroys
the window and bypasses that callback. `Show` and `Hide` can be requested
from another goroutine; their native operations run on the UI thread. A
closed window cannot be shown again; create a new window with a new manager.
`App.Post` is also safe before `Run`; queued work executes after startup.
Callbacks posted after `Quit` are discarded. `App.Run` is single-use.

`desktop.New(manager, options).Run()` still exits when its last window
closes. Use `App` when the process must outlive windows or own multiple
windows. Tray creation and menu behavior are not part of this API yet.

The Linux adapter multiplexes per-window X11/Wayland sockets and a wake pipe
on one locked UI thread. It currently uses `select` and reports an error if
the process gives it an FD outside its 1024-descriptor capacity. This is a
known portability limit for applications with unusually many open FDs.
