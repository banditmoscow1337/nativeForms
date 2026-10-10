package nativeforms

import (
	"testing"
	"time"
)

func scheduledAt(t *testing.T, manager *Manager, want time.Time) {
	t.Helper()
	got, ok := manager.NextFrameDeadline()
	if !ok || !got.Equal(want) {
		t.Fatalf("next frame deadline = %v, scheduled = %v; want %v", got, ok, want)
	}
}

func TestIdleDoesNotScheduleOrRenderAnotherFrame(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	manager := New()
	manager.clock = func() time.Time { return now }
	if !manager.NeedsFrame() {
		t.Fatal("initial UI frame was not requested")
	}
	manager.BeginFrame(320, 240, 0)
	if manager.NeedsFrame() {
		t.Fatal("idle manager requested another frame")
	}
	if deadline, ok := manager.NextFrameDeadline(); ok {
		t.Fatalf("idle manager has deadline %v", deadline)
	}
	now = now.Add(time.Hour)
	if manager.NeedsFrame() {
		t.Fatal("elapsed wall time requested a frame without a timer")
	}
}

func TestCaretBlinkRequestsFramesAtDeadlineOnly(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	manager := New()
	manager.clock = func() time.Time { return now }
	root := NewPanel()
	field := NewTextField("", nil)
	root.Add(field)
	manager.SetRoot(root)
	manager.setFocus(field)
	manager.BeginFrame(320, 240, 0)
	if !field.caretVisible || manager.NeedsFrame() {
		t.Fatal("focused field did not settle into an idle frame")
	}
	scheduledAt(t, manager, now.Add(500*time.Millisecond))
	now = now.Add(500*time.Millisecond - time.Nanosecond)
	if manager.NeedsFrame() {
		t.Fatal("caret requested a frame before its deadline")
	}
	now = now.Add(time.Nanosecond)
	if !manager.NeedsFrame() {
		t.Fatal("caret deadline did not request a frame without input")
	}
	manager.BeginFrame(320, 240, 0)
	if field.caretVisible || manager.NeedsFrame() {
		t.Fatal("caret did not blink and settle after the frame")
	}
	scheduledAt(t, manager, now.Add(500*time.Millisecond))
	now = now.Add(500 * time.Millisecond)
	manager.BeginFrame(320, 240, 0)
	if !field.caretVisible {
		t.Fatal("caret did not become visible on the next tick")
	}
	manager.setFocus(nil)
	manager.BeginFrame(320, 240, 0)
	if deadline, ok := manager.NextFrameDeadline(); ok {
		t.Fatalf("blur left a caret timer active at %v", deadline)
	}
}

func TestNotificationStartsAtNotifyAfterLongIdle(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	manager := New()
	manager.clock = func() time.Time { return now }
	manager.BeginFrame(320, 240, 0)
	now = now.Add(10 * time.Second)
	manager.Notify("fresh", 2500*time.Millisecond)
	scheduledAt(t, manager, now.Add(2500*time.Millisecond))
	manager.BeginFrame(320, 240, 10)
	if len(manager.notifications) != 1 || len(manager.Frame().Commands) == 0 {
		t.Fatal("new notification disappeared on its first frame")
	}
	if manager.NeedsFrame() {
		t.Fatal("notification requested continuous rendering before expiration")
	}
	now = now.Add(2500*time.Millisecond - time.Nanosecond)
	if manager.NeedsFrame() {
		t.Fatal("notification expired early")
	}
	now = now.Add(time.Nanosecond)
	if !manager.NeedsFrame() {
		t.Fatal("notification expiry did not request a frame")
	}
	manager.BeginFrame(320, 240, 0)
	if len(manager.notifications) != 0 || len(manager.Frame().Commands) != 0 {
		t.Fatal("expired notification remained on screen")
	}
	if manager.NeedsFrame() {
		t.Fatal("expired notification kept scheduling frames")
	}
}

func TestEarliestNotificationDeadlineDoesNotDelayTheNext(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	manager := New()
	manager.clock = func() time.Time { return now }
	manager.Notify("slow", 4*time.Second)
	manager.Notify("fast", 2*time.Second)
	manager.BeginFrame(320, 240, 0)
	scheduledAt(t, manager, now.Add(2*time.Second))
	now = now.Add(2 * time.Second)
	manager.BeginFrame(320, 240, 0)
	if len(manager.notifications) != 1 || manager.notifications[0].text != "slow" {
		t.Fatal("first expiry removed the later notification")
	}
	scheduledAt(t, manager, now.Add(2*time.Second))
	if manager.NeedsFrame() {
		t.Fatal("later notification caused an extra frame between deadlines")
	}
}

func TestNotificationRefreshAndSuspensionPreserveLifetime(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	manager := New()
	manager.clock = func() time.Time { return now }
	manager.Notify("save", 3*time.Second)
	manager.BeginFrame(320, 240, 0)
	now = now.Add(time.Second)
	manager.Notify("save", 3*time.Second)
	if len(manager.notifications) != 1 {
		t.Fatal("refresh duplicated the notification")
	}
	scheduledAt(t, manager, now.Add(3*time.Second))
	state := manager.SuspendNotifications()
	manager.BeginFrame(320, 240, 0)
	if deadline, ok := manager.NextFrameDeadline(); ok {
		t.Fatalf("suspension left a deadline active at %v", deadline)
	}
	now = now.Add(time.Minute)
	manager.RestoreNotifications(state)
	scheduledAt(t, manager, now.Add(3*time.Second))
	manager.BeginFrame(320, 240, 0)
	if len(manager.notifications) != 1 {
		t.Fatal("notification did not survive suspension")
	}
	now = now.Add(3 * time.Second)
	if !manager.NeedsFrame() {
		t.Fatal("restored notification did not expire")
	}
	manager.BeginFrame(320, 240, 0)
	if manager.NeedsFrame() {
		t.Fatal("restored notification left another frame due")
	}
}

func TestHiddenManagerDoesNotPollExpiredNotification(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	manager := New()
	manager.clock = func() time.Time { return now }
	manager.Notify("hidden", time.Second)
	manager.SetVisible(false)
	manager.BeginFrame(320, 240, 0)
	now = now.Add(2 * time.Second)
	if manager.NeedsFrame() {
		t.Fatal("invisible UI requested a frame for an expired notification")
	}
	if deadline, ok := manager.NextFrameDeadline(); ok {
		t.Fatalf("invisible UI scheduled a deadline at %v", deadline)
	}
	manager.SetVisible(true)
	manager.BeginFrame(320, 240, 0)
	if len(manager.notifications) != 0 || manager.NeedsFrame() {
		t.Fatal("expired hidden notification reappeared on resume")
	}
}

func TestPendingCallbackIsAppliedBeforeFrame(t *testing.T) {
	manager := New()
	manager.BeginFrame(320, 240, 0)
	called := false
	manager.Post(func() { called = true })
	if !manager.NeedsFrame() {
		t.Fatal("queued callback did not request a frame")
	}
	manager.BeginFrame(320, 240, 0)
	if !called || manager.NeedsFrame() {
		t.Fatal("queued callback did not run and settle before painting")
	}
}
