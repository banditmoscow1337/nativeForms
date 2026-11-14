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
		d.logf("System tray: enabled=%v (headless mode)", enabled)
		return
	}
	if !enabled {
		d.app.RemoveTray()
		d.logf("System tray icon removed (App.RemoveTray)")
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
			d.logf("Tray icon click #%d", d.trayClicks)
		},
		Menu: []ui.MenuItem{
			{ID: "show", Label: d.tr("plat.tray_show"), Action: func() {
				if d.window != nil {
					d.window.Show()
				}
			}},
			{ID: "hide", Label: d.tr("plat.tray_hide"), Action: func() {
				if d.window != nil {
					d.window.Hide()
				}
			}},
			{Separator: true},
			{ID: "theme", Label: d.tr("plat.tray_theme"), Shortcut: "F2", Action: d.toggleTheme},
			{ID: "lang", Label: d.tr("plat.tray_lang"), Shortcut: "F3", Action: d.toggleLanguage},
			{ID: "selftest", Label: d.tr("plat.tray_selftest"), Shortcut: "F5", Action: d.runSelfTests},
			{Separator: true},
			{ID: "quit", Label: d.tr("plat.tray_quit"), Shortcut: "Cmd+Q", Action: func() {
				if d.app != nil {
					d.app.Quit()
				}
			}},
		},
	})
	d.logf("System tray updated (App.SetTray): err=%v", err)
}

func (d *demoApp) setWindowFrameless(frameless bool) {
	if d.window != nil {
		d.window.SetFrameless(frameless)
	}
	d.logf("Window.SetFrameless: %v", frameless)
}

func (d *demoApp) setWindowControlsVisible(visible bool) {
	if d.window != nil {
		d.window.SetWindowControls(visible)
	}
	d.logf("Window.SetWindowControls: %v", visible)
}

func (d *demoApp) toggleSecondWindow() {
	if d.app == nil {
		d.logf("Secondary window is only available in desktop mode")
		return
	}
	if d.secondWin != nil {
		d.secondWin.Close()
		if d.subMgr != nil {
			d.subMgr.Close()
			d.subMgr = nil
		}
		d.secondWin = nil
		d.logf("Secondary window closed")
		return
	}
	subMgr := ui.New()
	if d.isLight {
		subMgr.SetTheme(d.lightTheme)
	} else {
		subMgr.SetTheme(d.darkTheme)
	}
	subMgr.SetFontContext(d.fonts)
	subMgr.SetLanguageManager(d.langMgr)
	subRoot := ui.NewStack(ui.Vertical, 10)
	subRoot.Padding = ui.All(16)
	subRoot.Background = subMgr.Theme().Panel
	subTitle := ui.NewLabelKey("plat.sub_title")
	subTitle.Font = ui.FontSpec{Size: 16, Weight: 700}
	subInfo := ui.NewLabelKey("plat.sub_info")
	subInfo.SetLocaleSizePolicy(ui.LocaleSizeWrapHeight)
	var subWin desktop.Window
	btnToggleSubFrame := ui.NewButton("", func() {
		if subWin != nil {
			subWin.SetFrameless(!subWin.Frameless())
		}
	})
	btnToggleSubFrame.SetTextKey("plat.sub_btn_frame")
	btnCloseSub := ui.NewButton("", func() {
		if subWin != nil {
			subWin.Close()
			subMgr.Close()
			d.subMgr = nil
			d.secondWin = nil
		}
	})
	btnCloseSub.SetTextKey("plat.sub_btn_close")
	subRoot.Add(subTitle, subInfo, btnToggleSubFrame, btnCloseSub)
	subMgr.SetRoot(subRoot)

	w, err := d.app.NewWindow(subMgr, desktop.Options{
		Title:    d.tr("app.second_title"),
		Width:    460,
		Height:   260,
		Graphics: render.Kind(d.flags.graphics),
		OnCloseRequest: func(w desktop.Window) platform.CloseAction {
			subMgr.Close()
			d.subMgr = nil
			d.secondWin = nil
			return platform.CloseWindow
		},
	})
	if err != nil {
		subMgr.Close()
		d.logf("Error opening secondary window: %v", err)
		return
	}
	subWin = w
	d.subMgr = subMgr
	d.secondWin = w
	d.logf("Secondary window opened (App.NewWindow)")
}

func (d *demoApp) buildPopupsAndPlatformTab() ui.Component {
	columns := ui.NewStack(ui.Horizontal, 12)
	columns.Padding = ui.All(8)

	left := ui.NewStack(ui.Vertical, 8)
	left.UIState().SetFlex(1)

	lblPopup := newSectionHeaderKey("plat.sec_popups")

	btnModal := ui.NewButton("", func() {
		var popupID ui.PopupID
		body := ui.NewLabelKey("plat.dlg_body")
		body.SetLocaleSizePolicy(ui.LocaleSizeWrapHeight)
		okBtn := ui.NewButtonKey("plat.dlg_confirm", func() {
			d.manager.ClosePopup(popupID)
			d.manager.Notify(d.tr("plat.dlg_confirmed"))
		})
		okBtn.SetIcon(d.icons[2])
		cancelBtn := ui.NewButtonKey("plat.dlg_cancel", func() {
			d.manager.ClosePopup(popupID)
		})
		dlg := ui.NewDialog(d.tr("plat.dlg_title"), body, okBtn, cancelBtn)
		popupID = d.manager.OpenDialog(dlg, 460, 220)
	})
	btnModal.SetTextKey("plat.btn_modal")

	menuRow := ui.NewStack(ui.Horizontal, 6)
	btnMenu := newFlexButtonKey("plat.btn_ctx_menu", func() {
		b := d.tabs.Bounds()
		d.manager.OpenContextMenu([]ui.MenuItem{
			{ID: "notify", LabelKey: "plat.ctx_notify", Icon: d.icons[0], Shortcut: "Ctrl+N", Action: func() { d.manager.Notify(d.tr("plat.ctx_notify_msg")) }},
			{ID: "newtab", LabelKey: "plat.ctx_newtab", Icon: d.icons[1], Shortcut: "Ctrl+T", Action: d.openDynamicTab},
			{Separator: true},
			{ID: "checked", LabelKey: "plat.ctx_checked", Checked: true, Action: func() { d.logf("Checked item selected") }},
			{ID: "disabled", LabelKey: "plat.ctx_disabled", Icon: d.icons[3], Disabled: true},
			{ID: "clear", LabelKey: "plat.ctx_clear", Icon: d.icons[2], Action: func() { d.manager.ClearNotifications() }},
		}, b.X+120, b.Y+120)
	})

	var btnAnchoredMenu *ui.Button
	btnAnchoredMenu = newFlexButtonKey("plat.btn_anchored", func() {
		d.manager.OpenMenu([]ui.MenuItem{
			{ID: "m1", LabelKey: "plat.anc_item1", Icon: d.icons[0], Action: func() { d.logf("OpenMenu: item #1") }},
			{ID: "m2", LabelKey: "plat.anc_item2", Icon: d.icons[1], Action: func() { d.logf("OpenMenu: item #2") }},
		}, btnAnchoredMenu.Bounds())
	})
	menuRow.Add(btnMenu, btnAnchoredMenu)

	popupRow := ui.NewStack(ui.Horizontal, 6)
	var btnUnfocused *ui.Button
	btnUnfocused = newFlexButtonKey("plat.btn_unfocused", func() {
		if d.unfocusedPopupID != 0 && d.manager.ClosePopup(d.unfocusedPopupID) {
			d.unfocusedPopupID = 0
			return
		}
		card := ui.NewStack(ui.Vertical, 6)
		card.Padding = ui.All(10)
		card.Background = d.manager.Theme().PanelAlt
		card.Border = d.manager.Theme().Accent
		card.BorderWidth = 1
		lbl := ui.NewLabelKey("plat.unfocused_body")
		lbl.SetLocaleSizePolicy(ui.LocaleSizeWrapHeight)
		closeBtn := ui.NewButton("", func() {
			d.manager.ClosePopup(d.unfocusedPopupID)
			d.unfocusedPopupID = 0
		})
		closeBtn.SetTextKey("plat.unfocused_close")
		card.Add(lbl, closeBtn)
		b := btnUnfocused.Bounds()
		d.unfocusedPopupID = d.manager.OpenUnfocusedPopup(card, ui.PopupOptions{
			Bounds: ui.Rect{X: b.X, Y: b.Y + b.H + 4, W: 300, H: 110},
		})
	})

	btnTooltip := newFlexButtonKey("plat.btn_tooltip", func() {
		d.manager.ShowTooltip(d.tr("plat.tooltip_text"), ui.Rect{X: 280, Y: 180, W: 360, H: 40})
	})
	popupRow.Add(btnUnfocused, btnTooltip)

	notifRow := ui.NewStack(ui.Horizontal, 6)
	btnNotify := newFlexButtonKey("plat.btn_notify", func() {
		d.manager.ShowMessage(d.tr("plat.notify_fmt"), 3.0)
	})

	btnSuspend := newFlexButtonKey("plat.btn_suspend", func() {
		if d.suspendedNotif == nil {
			state := d.manager.SuspendNotifications()
			d.suspendedNotif = &state
			d.logf("Notifications suspended (SuspendNotifications)")
		} else {
			d.manager.RestoreNotifications(*d.suspendedNotif)
			d.suspendedNotif = nil
			d.logf("Notifications restored (RestoreNotifications)")
		}
	})
	notifRow.Add(btnNotify, btnSuspend)

	tabRow := ui.NewStack(ui.Horizontal, 6)
	btnDynTab := newFlexButtonKey("plat.btn_tab_open", d.openDynamicTab)
	btnMoveTab := newFlexButtonKey("plat.btn_tab_move", func() {
		if len(d.tabs.Tabs) > 5 {
			lastID := d.tabs.Tabs[len(d.tabs.Tabs)-1].ID
			d.tabs.Move(lastID, 1)
			d.logf("Tab %s moved to index 1", lastID)
		} else {
			d.openDynamicTab()
			lastID := d.tabs.Tabs[len(d.tabs.Tabs)-1].ID
			d.tabs.Move(lastID, 1)
		}
	})
	tabRow.Add(btnDynTab, btnMoveTab)

	lblTrayWin := newSectionHeaderKey("plat.sec_tray")

	d.trayToggle = ui.NewToggleKey("plat.tgl_tray", true, func(v bool) {
		d.setTrayEnabled(v)
	})
	d.trayToggle.SetIcon(d.icons[2])

	d.winFramelessToggle = ui.NewToggleKey("plat.tgl_win_frame", d.flags.frameless, func(v bool) {
		d.setWindowFrameless(v)
	})
	d.winFramelessToggle.SetIcon(d.icons[0])

	d.winControlsToggle = ui.NewToggleKey("plat.tgl_win_controls", !d.flags.hideWindowControls, func(v bool) {
		d.setWindowControlsVisible(v)
	})
	d.winControlsToggle.SetIcon(d.icons[1])

	winActionRow := ui.NewStack(ui.Horizontal, 6)
	btnMin := newFlexButtonKey("Minimize()", func() {
		if d.window != nil {
			d.window.Minimize()
		}
		d.logf("Window.Minimize() called")
	})

	btnMax := newFlexButtonKey("Maximize()", func() {
		if d.window != nil {
			d.window.Maximize()
		}
		d.logf("Window.Maximize() called")
	})

	btnHideShow := newFlexButtonKey("plat.btn_hide_show", func() {
		if d.window != nil {
			d.window.Hide()
			time.AfterFunc(time.Second, func() {
				if d.window != nil {
					d.window.Show()
				}
			})
		}
		d.logf("Window.Hide() called with 1s auto Show()")
	})

	btnSecondWin := newFlexButtonKey("plat.btn_second_win", d.toggleSecondWindow)
	winActionRow.Add(btnMin, btnMax, btnHideShow, btnSecondWin)

	left.Add(
		lblPopup, btnModal, menuRow, popupRow, notifRow, tabRow,
		lblTrayWin, d.trayToggle, d.winFramelessToggle, d.winControlsToggle, winActionRow,
	)

	right := ui.NewStack(ui.Vertical, 8)
	right.UIState().SetFlex(1)

	lblPlat := newSectionHeaderKey("plat.sec_policy")

	d.closePolicyRadio = ui.NewRadioGroup([]ui.RadioOption{
		{ID: "quit", LabelKey: "plat.pol_quit", Icon: d.icons[3]},
		{ID: "hide", LabelKey: "plat.pol_hide", Icon: d.icons[2]},
		{ID: "cancel", LabelKey: "plat.pol_cancel", Icon: d.icons[0]},
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
		d.logf("Close policy set to: %s", id)
	})

	d.a11ySummary = ui.NewLabelKey("plat.a11y_hint")
	d.a11ySummary.SetLocaleSizePolicy(ui.LocaleSizeWrapHeight)

	btnA11y := ui.NewButton("", func() {
		tree := d.manager.AccessibilitySnapshot()
		roleCounts := make(map[ui.AccessibleRole]int)
		for _, node := range tree.Nodes {
			roleCounts[node.Role]++
		}
		sliderID := d.sliderStep.UIState().AccessibilityID()
		acted := d.manager.PerformAccessibilityAction(sliderID, ui.AccessibilityIncrement, "")
		summary := d.trf(
			"plat.a11y_fmt",
			len(tree.Roots), len(tree.Nodes), tree.Focused,
			roleCounts[ui.RoleButton], roleCounts[ui.RoleTextField], roleCounts[ui.RoleSlider], roleCounts[ui.RoleMenu], acted,
		)
		d.a11ySummary.SetText(summary)
		d.logf("%s", summary)
	})
	btnA11y.SetTextKey("plat.btn_a11y")

	d.clipField = ui.NewTextField("", nil)
	d.clipField.SetTextKey("plat.clip_init")
	d.scopedShortcutID = d.manager.AddShortcut(ui.Shortcut{
		Key:   ui.KeyK,
		Mods:  ui.ModControl,
		Scope: d.clipField,
		Action: func() {
			d.clipField.SetText("")
			d.logf("Scoped shortcut (Ctrl+K) cleared clipboard field")
		},
	})

	clipRow := ui.NewStack(ui.Horizontal, 6)
	btnCopy := newFlexButtonKey("plat.btn_copy", func() {
		if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
			err := d.window.SetClipboardText(d.clipField.Text)
			d.logf("SetClipboardText: err=%v", err)
		} else {
			d.logf("Platform clipboard unavailable in current mode")
		}
	})

	btnPaste := newFlexButtonKey("plat.btn_paste", func() {
		if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
			txt, err := d.window.ClipboardText()
			if err == nil {
				d.clipField.SetText(txt)
			}
			d.logf("ClipboardText: %q (err=%v)", txt, err)
		} else {
			d.logf("Platform clipboard unavailable in current mode")
		}
	})
	clipRow.Add(btnCopy, btnPaste)

	dialogRow := ui.NewStack(ui.Horizontal, 6)
	btnOpenFile := ui.NewButton("OpenFile()", func() {
		if d.window == nil {
			d.logf("OpenFile skipped (headless mode)")
			return
		}
		path, err := d.window.OpenFile()
		d.logf("OpenFile: path=%q err=%v", path, err)
	})
	btnOpenFile.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnOpenFile.UIState().SetFlex(1)

	btnOpenFolder := ui.NewButton("OpenFolder()", func() {
		if d.window == nil {
			d.logf("OpenFolder skipped (headless mode)")
			return
		}
		path, err := d.window.OpenFolder()
		d.logf("OpenFolder: path=%q err=%v", path, err)
	})
	btnOpenFolder.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnOpenFolder.UIState().SetFlex(1)

	btnSaveFile := ui.NewButton("SaveFile()", func() {
		if d.window == nil {
			d.logf("SaveFile skipped (headless mode)")
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
			return d.tr("plat.cap_headless")
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
