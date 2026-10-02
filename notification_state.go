package nativeforms

type NotificationState struct{ items []notification }

func (manager *Manager) SuspendNotifications() NotificationState {
	manager.notificationMu.Lock()
	defer manager.notificationMu.Unlock()
	state := NotificationState{items: manager.notifications}
	manager.notifications = nil
	return state
}
func (manager *Manager) RestoreNotifications(state NotificationState) {
	manager.notificationMu.Lock()
	defer manager.notificationMu.Unlock()
	manager.notifications = append([]notification(nil), state.items...)
}
