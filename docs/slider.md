# Slider

The horizontal `Slider` already exists in the `nativeforms` package. Use it as a volume control by mapping its value to your audio backend in `OnChange`:

```go
volume := nativeforms.NewSlider("Громкость", 0, 100, 35, 1, func(value float32) {
    fmt.Printf("volume: %.0f%%\n", value)
    // audio.SetVolume(value / 100)
})
volume.FormatValue = func(value float32) string {
    return fmt.Sprintf("%.0f%%", value)
}

manager := nativeforms.New()
manager.SetRoot(volume)
manager.SetInteractive(true)
```

The constructor takes a label, minimum, maximum, initial value, step, and change callback. `SetValue` clamps and snaps the value and invokes `OnChange` only when it actually changes. Positive steps start at the minimum; the exact maximum remains reachable even when the range is not divisible by the step. A step of zero allows continuous pointer input.

Drag with the left mouse button or click the track. When focused, use Left/Down to decrease, Right/Up to increase, and Home/End to jump to the bounds. Continuous sliders move by one twentieth of their range per key press. The accessibility tree exposes the value and range and supports increment, decrement, and setting the value.
