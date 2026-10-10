package nativeforms

import "time"

type suspendedNotification struct {
	text      string
	remaining time.Duration
}

// NotificationState holds notification lifetimes while they are suspended.
type NotificationState struct{ items []suspendedNotification }

func (manager *Manager) SuspendNotifications() NotificationState {
	now := manager.now()
	manager.notificationMu.Lock()
	state := NotificationState{}
	for _, item := range manager.notifications {
		if remaining := item.deadline.Sub(now); remaining > 0 {
			state.items = append(state.items, suspendedNotification{item.text, remaining})
		}
	}
	manager.notifications = nil
	manager.notificationMu.Unlock()
	manager.InvalidatePaint()
	return state
}
func (manager *Manager) RestoreNotifications(state NotificationState) {
	now := manager.now()
	manager.notificationMu.Lock()
	manager.notifications = manager.notifications[:0]
	for _, item := range state.items {
		manager.notifications = append(manager.notifications, notification{item.text, now.Add(item.remaining)})
	}
	manager.notificationMu.Unlock()
	manager.InvalidatePaint()
}
