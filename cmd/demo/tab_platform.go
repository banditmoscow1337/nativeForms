package main

import (
	"fmt"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
	"github.com/banditmoscow1337/nativeForms/render"
)

func (d *demoApp) setTrayEnabled(enabled bool) {
	d.trayActive = enabled
	if d.trayToggle != nil && d.trayToggle.Value != enabled {
		d.trayToggle.SetValue(enabled)
	}
	if d.app == nil {
		d.logf("Системный трей: состояние=%v (headless режим)", enabled)
		return
	}
	if !enabled {
		d.app.RemoveTray()
		d.logf("Иконка приложения удалена из системного трея (App.RemoveTray)")
		return
	}
	err := d.app.SetTray(desktop.TrayOptions{
		Title:   "NF",
		Tooltip: "nativeForms Showcase & Profiler",
		Icon:    d.icons[0],
		OnClick: func() {
			d.trayClicks++
			if d.window != nil {
				d.window.Show()
			}
			d.logf("Клик по иконке трея #%d", d.trayClicks)
		},
		Menu: []ui.MenuItem{
			{ID: "show", Label: "Показать главное окно", Action: func() {
				if d.window != nil {
					d.window.Show()
				}
			}},
			{ID: "hide", Label: "Скрыть главное окно (Hide)", Action: func() {
				if d.window != nil {
					d.window.Hide()
				}
			}},
			{Separator: true},
			{ID: "theme", Label: "Переключить тему Dark/Light", Shortcut: "F2", Action: d.toggleTheme},
			{ID: "selftest", Label: "Запустить самопроверку API", Shortcut: "F5", Action: d.runSelfTests},
			{Separator: true},
			{ID: "quit", Label: "Выход из приложения", Shortcut: "Cmd+Q", Action: func() {
				if d.app != nil {
					d.app.Quit()
				}
			}},
		},
	})
	d.logf("Системный трей установлен (App.SetTray): err=%v", err)
}

func (d *demoApp) setWindowFrameless(frameless bool) {
	if d.window != nil {
		d.window.SetFrameless(frameless)
	}
	d.logf("Окно без стандартной рамки ОС (Window.SetFrameless): %v", frameless)
}

func (d *demoApp) setWindowControlsVisible(visible bool) {
	if d.window != nil {
		d.window.SetWindowControls(visible)
	}
	d.logf("Стандартные кнопки управления окном (Window.SetWindowControls): %v", visible)
}

func (d *demoApp) toggleSecondWindow() {
	if d.app == nil {
		d.logf("Дополнительное окно доступно только в desktop-режиме")
		return
	}
	if d.secondWin != nil {
		d.secondWin.Close()
		d.secondWin = nil
		d.logf("Дополнительное окно закрыто")
		return
	}
	subMgr := ui.New()
	if d.isLight {
		subMgr.SetTheme(d.lightTheme)
	} else {
		subMgr.SetTheme(d.darkTheme)
	}
	subMgr.SetFontContext(d.fonts)
	subRoot := ui.NewStack(ui.Vertical, 10)
	subRoot.Padding = ui.All(16)
	subRoot.Background = subMgr.Theme().Panel
	subTitle := ui.NewLabel("Дополнительное окно (MultipleWindows / App.NewWindow)")
	subTitle.Font = ui.FontSpec{Size: 16, Weight: 700}
	subInfo := ui.NewLabel("У каждого окна собственный ui.Manager, независимый фокус и поддержка Frameless-режима.")
	subInfo.Wrap = true
	var subWin desktop.Window
	btnToggleSubFrame := ui.NewButton("Переключить рамку этого окна (SetFrameless)", func() {
		if subWin != nil {
			subWin.SetFrameless(!subWin.Frameless())
		}
	})
	btnCloseSub := ui.NewButton("Закрыть дополнительное окно", func() {
		if subWin != nil {
			subWin.Close()
			subMgr.Close()
			d.secondWin = nil
		}
	})
	subRoot.Add(subTitle, subInfo, btnToggleSubFrame, btnCloseSub)
	subMgr.SetRoot(subRoot)

	w, err := d.app.NewWindow(subMgr, desktop.Options{
		Title:    "nativeForms — Второе окно",
		Width:    460,
		Height:   260,
		Graphics: render.Kind(d.flags.graphics),
		OnCloseRequest: func(w desktop.Window) platform.CloseAction {
			subMgr.Close()
			d.secondWin = nil
			return platform.CloseWindow
		},
	})
	if err != nil {
		d.logf("Ошибка открытия второго окна: %v", err)
		return
	}
	subWin = w
	d.secondWin = w
	d.logf("Открыто дополнительное окно (App.NewWindow)")
}

func (d *demoApp) buildPopupsAndPlatformTab() ui.Component {
	columns := ui.NewStack(ui.Horizontal, 12)
	columns.Padding = ui.All(8)

	left := ui.NewStack(ui.Vertical, 8)
	left.UIState().SetFlex(1)

	lblPopup := newSectionHeader("Оверлеи: Dialog, ContextMenu, OpenMenu, UnfocusedPopup, Tooltip, Notifications")

	btnModal := ui.NewButton("Открыть модальный диалог (OpenDialog / NewDialog)", func() {
		var popupID ui.PopupID
		body := ui.NewLabel("Модальное окно блокирует клики снаружи и фокусирует вложенные кнопки. Нажмите Escape или кнопку ниже для закрытия.")
		body.Wrap = true
		okBtn := ui.NewButton("Подтвердить", func() {
			d.manager.ClosePopup(popupID)
			d.manager.Notify("Модальный диалог подтверждён")
		})
		okBtn.SetIcon(d.icons[2])
		cancelBtn := ui.NewButton("Отмена", func() {
			d.manager.ClosePopup(popupID)
		})
		dlg := ui.NewDialog("Проверка модального диалога (RoleDialog)", body, okBtn, cancelBtn)
		popupID = d.manager.OpenDialog(dlg, 460, 220)
	})

	menuRow := ui.NewStack(ui.Horizontal, 6)
	btnMenu := newFlexButton("OpenContextMenu", func() {
		b := d.tabs.Bounds()
		d.manager.OpenContextMenu([]ui.MenuItem{
			{ID: "notify", Label: "Показать уведомление", Icon: d.icons[0], Shortcut: "Ctrl+N", Action: func() { d.manager.Notify("Выбрано действие из меню!") }},
			{ID: "newtab", Label: "Открыть новую вкладку", Icon: d.icons[1], Shortcut: "Ctrl+T", Action: d.openDynamicTab},
			{Separator: true},
			{ID: "checked", Label: "Пункт с галочкой (Checked)", Checked: true, Action: func() { d.logf("Выбран Checked пункт") }},
			{ID: "disabled", Label: "Неактивный пункт (Disabled)", Icon: d.icons[3], Disabled: true},
			{ID: "clear", Label: "Очистить уведомления", Icon: d.icons[2], Action: func() { d.manager.ClearNotifications() }},
		}, b.X+120, b.Y+120)
	})

	var btnAnchoredMenu *ui.Button
	btnAnchoredMenu = ui.NewButton("OpenMenu (у кнопки)", func() {
		d.manager.OpenMenu([]ui.MenuItem{
			{ID: "m1", Label: "Привязанное меню #1", Icon: d.icons[0], Action: func() { d.logf("OpenMenu: пункт #1") }},
			{ID: "m2", Label: "Привязанное меню #2", Icon: d.icons[1], Action: func() { d.logf("OpenMenu: пункт #2") }},
		}, btnAnchoredMenu.Bounds())
	})
	btnAnchoredMenu.UIState().SetFlex(1)
	menuRow.Add(btnMenu, btnAnchoredMenu)

	popupRow := ui.NewStack(ui.Horizontal, 6)
	var btnUnfocused *ui.Button
	btnUnfocused = ui.NewButton("OpenUnfocusedPopup (инспектор)", func() {
		if d.unfocusedPopupID != 0 && d.manager.ClosePopup(d.unfocusedPopupID) {
			d.unfocusedPopupID = 0
			return
		}
		card := ui.NewStack(ui.Vertical, 6)
		card.Padding = ui.All(10)
		card.Background = d.manager.Theme().PanelAlt
		card.Border = d.manager.Theme().Accent
		card.BorderWidth = 1
		lbl := ui.NewLabel("Плавающий попап без перехвата фокуса ввода (OpenUnfocusedPopup)")
		lbl.Wrap = true
		closeBtn := ui.NewButton("Закрыть попап", func() {
			d.manager.ClosePopup(d.unfocusedPopupID)
			d.unfocusedPopupID = 0
		})
		card.Add(lbl, closeBtn)
		b := btnUnfocused.Bounds()
		d.unfocusedPopupID = d.manager.OpenUnfocusedPopup(card, ui.PopupOptions{
			Bounds: ui.Rect{X: b.X, Y: b.Y + b.H + 4, W: 300, H: 110},
		})
	})
	btnUnfocused.UIState().SetFlex(1)

	btnTooltip := newFlexButton("ShowTooltip", func() {
		d.manager.ShowTooltip("Пассивный тултип (пропускает клики сквозь себя)", ui.Rect{X: 280, Y: 180, W: 360, H: 40})
	})
	popupRow.Add(btnUnfocused, btnTooltip)

	notifRow := ui.NewStack(ui.Horizontal, 6)
	btnNotify := newFlexButton("Notify / ShowMessage", func() {
		d.manager.ShowMessage("Тост-уведомление (%f сек)", 3.0)
	})

	btnSuspend := newFlexButton("Suspend / Restore Notifications", func() {
		if d.suspendedNotif == nil {
			state := d.manager.SuspendNotifications()
			d.suspendedNotif = &state
			d.logf("Уведомления приостановлены (SuspendNotifications)")
		} else {
			d.manager.RestoreNotifications(*d.suspendedNotif)
			d.suspendedNotif = nil
			d.logf("Уведомления восстановлены (RestoreNotifications)")
		}
	})
	notifRow.Add(btnNotify, btnSuspend)

	tabRow := ui.NewStack(ui.Horizontal, 6)
	btnDynTab := newFlexButton("Tabs.Open (новая вкладка)", d.openDynamicTab)
	btnMoveTab := newFlexButton("Tabs.Move (сдвинуть вкладку)", func() {
		if len(d.tabs.Tabs) > 5 {
			lastID := d.tabs.Tabs[len(d.tabs.Tabs)-1].ID
			d.tabs.Move(lastID, 1)
			d.logf("Вкладка %s перемещена на индекс 1", lastID)
		} else {
			d.openDynamicTab()
			lastID := d.tabs.Tabs[len(d.tabs.Tabs)-1].ID
			d.tabs.Move(lastID, 1)
		}
	})
	tabRow.Add(btnDynTab, btnMoveTab)

	// System Tray & Window Frame / Controls section (Issues #4 and #6)
	lblTrayWin := newSectionHeader("Системный трей (App.SetTray) и Управление рамкой/кнопками окна (Window)")

	d.trayToggle = ui.NewToggle("Иконка в системном трее / строке меню macOS (App.SetTray / RemoveTray)", true, func(v bool) {
		d.setTrayEnabled(v)
	})
	d.trayToggle.SetIcon(d.icons[2])

	d.winFramelessToggle = ui.NewToggle("Отключить стандартную рамку и заголовок окна ОС (Window.SetFrameless)", d.flags.frameless, func(v bool) {
		d.setWindowFrameless(v)
	})
	d.winFramelessToggle.SetIcon(d.icons[0])

	d.winControlsToggle = ui.NewToggle("Показывать стандартные кнопки управления окном (Window.SetWindowControls)", !d.flags.hideWindowControls, func(v bool) {
		d.setWindowControlsVisible(v)
	})
	d.winControlsToggle.SetIcon(d.icons[1])

	winActionRow := ui.NewStack(ui.Horizontal, 6)
	btnMin := newFlexButton("Minimize()", func() {
		if d.window != nil {
			d.window.Minimize()
		}
		d.logf("Вызван Window.Minimize()")
	})

	btnMax := newFlexButton("Maximize()", func() {
		if d.window != nil {
			d.window.Maximize()
		}
		d.logf("Вызван Window.Maximize()")
	})

	btnHideShow := newFlexButton("Hide() на 1 сек -> Show()", func() {
		if d.window != nil {
			d.window.Hide()
			time.AfterFunc(time.Second, func() {
				if d.window != nil {
					d.window.Show()
				}
			})
		}
		d.logf("Вызван Window.Hide() с автоматическим Window.Show() через 1с")
	})

	btnSecondWin := newFlexButton("Второе окно (NewWindow)", d.toggleSecondWindow)
	winActionRow.Add(btnMin, btnMax, btnHideShow, btnSecondWin)

	left.Add(
		lblPopup, btnModal, menuRow, popupRow, notifRow, tabRow,
		lblTrayWin, d.trayToggle, d.winFramelessToggle, d.winControlsToggle, winActionRow,
	)

	right := ui.NewStack(ui.Vertical, 8)
	right.UIState().SetFlex(1)

	lblPlat := newSectionHeader("Политика закрытия окна (OnCloseRequest), Accessibility и Буфер обмена")

	d.closePolicyRadio = ui.NewRadioGroup([]ui.RadioOption{
		{ID: "quit", Label: "OnCloseRequest: platform.QuitApp (завершить приложение)", Icon: d.icons[3]},
		{ID: "hide", Label: "OnCloseRequest: platform.HideWindow (свернуть в системный трей)", Icon: d.icons[2]},
		{ID: "cancel", Label: "OnCloseRequest: platform.CancelClose (заблокировать закрытие)", Icon: d.icons[0]},
	}, "quit", func(id string) {
		switch id {
		case "quit":
			d.closePolicy = platform.QuitApp
		case "hide":
			d.closePolicy = platform.HideWindow
			if !d.trayActive {
				d.setTrayEnabled(true)
			}
		case "cancel":
			d.closePolicy = platform.CancelClose
		}
		d.logf("Установлена политика закрытия окна: %s", id)
	})

	d.a11ySummary = ui.NewLabel("Нажмите «Снимок AccessibilitySnapshot», чтобы проинспектировать дерево доступности.")
	d.a11ySummary.Wrap = true

	btnA11y := ui.NewButton("Снимок AccessibilitySnapshot + PerformAccessibilityAction", func() {
		tree := d.manager.AccessibilitySnapshot()
		roleCounts := make(map[ui.AccessibleRole]int)
		for _, node := range tree.Nodes {
			roleCounts[node.Role]++
		}
		sliderID := d.sliderStep.UIState().AccessibilityID()
		acted := d.manager.PerformAccessibilityAction(sliderID, ui.AccessibilityIncrement, "")
		summary := fmt.Sprintf(
			"AccessibilityTree: корней=%d, узлов=%d, фокус ID=%d | Button=%d, TextField=%d, Slider=%d, Menu=%d | PerformAction(Slider++): %v",
			len(tree.Roots), len(tree.Nodes), tree.Focused,
			roleCounts[ui.RoleButton], roleCounts[ui.RoleTextField], roleCounts[ui.RoleSlider], roleCounts[ui.RoleMenu], acted,
		)
		d.a11ySummary.SetText(summary)
		d.logf("%s", summary)
	})

	d.clipField = ui.NewTextField("Текст для проверки буфера обмена UTF-8 (Ctrl+K в этом поле очищает текст)", nil)
	d.scopedShortcutID = d.manager.AddShortcut(ui.Shortcut{
		Key:   ui.KeyK,
		Mods:  ui.ModControl,
		Scope: d.clipField,
		Action: func() {
			d.clipField.SetText("")
			d.logf("Сработал локальный Shortcut (Ctrl+K) внутри поля буфера обмена")
		},
	})

	clipRow := ui.NewStack(ui.Horizontal, 6)
	btnCopy := newFlexButton("В буфер (SetClipboardText)", func() {
		if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
			err := d.window.SetClipboardText(d.clipField.Text)
			d.logf("SetClipboardText: err=%v", err)
		} else {
			d.logf("Платформенный буфер обмена недоступен в текущем режиме")
		}
	})

	btnPaste := newFlexButton("Из буфера (ClipboardText)", func() {
		if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
			txt, err := d.window.ClipboardText()
			if err == nil {
				d.clipField.SetText(txt)
			}
			d.logf("ClipboardText: %q (err=%v)", txt, err)
		} else {
			d.logf("Платформенный буфер обмена недоступен в текущем режиме")
		}
	})
	clipRow.Add(btnCopy, btnPaste)

	dialogRow := ui.NewStack(ui.Horizontal, 6)
	btnOpenFile := ui.NewButton("OpenFile()", func() {
		if d.window == nil {
			d.logf("OpenFile пропущен (headless режим)")
			return
		}
		path, err := d.window.OpenFile()
		d.logf("OpenFile: path=%q err=%v", path, err)
	})
	btnOpenFile.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnOpenFile.UIState().SetFlex(1)

	btnOpenFolder := ui.NewButton("OpenFolder()", func() {
		if d.window == nil {
			d.logf("OpenFolder пропущен (headless режим)")
			return
		}
		path, err := d.window.OpenFolder()
		d.logf("OpenFolder: path=%q err=%v", path, err)
	})
	btnOpenFolder.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnOpenFolder.UIState().SetFlex(1)

	btnSaveFile := ui.NewButton("SaveFile()", func() {
		if d.window == nil {
			d.logf("SaveFile пропущен (headless режим)")
			return
		}
		path, err := d.window.SaveFile("report.jsonl")
		d.logf("SaveFile: path=%q err=%v", path, err)
	})
	btnSaveFile.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnSaveFile.UIState().SetFlex(1)
	dialogRow.Add(btnOpenFile, btnOpenFolder, btnSaveFile)

	var cachedCapText string
	d.capSummary = ui.NewLabel("")
	d.capSummary.Wrap = true
	d.capSummary.TextFunc = func() string {
		if d.window == nil {
			return "Capabilities: Headless Software Renderer (без нативного окна ОС)"
		}
		if cachedCapText == "" {
			c := d.window.Capabilities()
			cachedCapText = fmt.Sprintf(
				"Capabilities: SoftwareFrame=%v | Pointer=%v | Keyboard=%v | TextInput=%v | Clipboard=%v | IME=%v | MultiWin=%v | DragDrop=%v | FileDialog=%v | Tray=%v | Frameless=%v | Wayland=%v",
				c.SoftwareFrame, c.Pointer, c.Keyboard, c.TextInput,
				c.Clipboard, c.IME, c.MultipleWindows, c.DragDrop, c.FileDialog, c.Tray, c.Frameless, c.Wayland,
			)
		}
		return cachedCapText
	}

	right.Add(lblPlat, d.closePolicyRadio, btnA11y, d.a11ySummary, d.clipField, clipRow, dialogRow, d.capSummary)

	leftScroll := ui.NewScrollPanel(left)
	leftScroll.UIState().SetFlex(1)
	rightScroll := ui.NewScrollPanel(right)
	rightScroll.UIState().SetFlex(1)
	columns.Add(leftScroll, rightScroll)
	return columns
}

