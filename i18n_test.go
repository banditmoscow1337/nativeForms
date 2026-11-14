package nativeforms

import (
	"testing"
)

func TestLanguageManagerAndSizingPolicies(t *testing.T) {
	lm := NewLanguageManager("en")
	lm.Register("en", map[string]string{
		"btn.action":  "OK",
		"btn.tooltip": "Confirm action",
		"lbl.field":   "Name:",
		"tf.hint":     "Enter name...",
		"menu.file":   "File",
		"menu.open":   "Open...",
		"tab.main":    "Controls",
		"opt.fit":     "Contain",
	})
	lm.Register("ru", map[string]string{
		"btn.action":  "Подтвердить выбранное действие",
		"btn.tooltip": "Подтвердить действие",
		"lbl.field":   "Имя пользователя в системе:",
		"tf.hint":     "Введите полное имя пользователя...",
		"menu.file":   "Файл",
		"menu.open":   "Открыть файл...",
		"tab.main":    "Виджеты и Формы",
		"opt.fit":     "Вписать в рамку (Contain)",
	})

	if got := lm.Tr("btn.action"); got != "OK" {
		t.Fatalf("expected EN 'OK', got %q", got)
	}
	if got := lm.TrLocale("ru", "btn.action"); got != "Подтвердить выбранное действие" {
		t.Fatalf("expected RU translation, got %q", got)
	}
	if got := lm.Tr("missing.key"); got != "missing.key" {
		t.Fatalf("expected fallback to key for missing translation, got %q", got)
	}

	mgr := New()
	defer mgr.Close()
	mgr.SetLanguageManager(lm)

	root := NewStack(Vertical, 8)
	btnMax := NewButtonKey("btn.action", nil)
	btnMax.SetTooltipKey("btn.tooltip")

	btnCur := NewCompactButton("", nil)
	btnCur.SetTextKey("btn.action")
	btnCur.SetLocaleSizePolicy(LocaleSizeCurrent)

	tf := NewTextField("", nil)
	tf.SetPlaceholderKey("tf.hint")
	tf.SetLocaleSizePolicy(LocaleSizeMaxEnvelope)

	form := NewLocalizedForm()
	lblRow := form.AddLocalizedRow("lbl.field", tf)

	menuBar := NewMenuBar(MenuBarItem{
		ID:       "file",
		LabelKey: "menu.file",
		Items: []MenuItem{
			{ID: "open", LabelKey: "menu.open"},
		},
	})

	tabs := NewTabs([]Tab{
		{ID: "main", TitleKey: "tab.main", Content: form},
	}, "main", nil)

	seg := NewSegmentedControl([]RadioOption{
		{ID: "contain", LabelKey: "opt.fit"},
	}, "contain", nil)
	seg.SetLocaleSizePolicy(LocaleSizeMaxEnvelope)

	root.Add(menuBar, btnMax, btnCur, seg, tabs)
	mgr.SetRoot(root)
	mgr.BeginFrame(800, 600, 0.016)

	if btnMax.Text != "OK" || btnMax.Tooltip() != "Confirm action" {
		t.Fatalf("unexpected EN button state: text=%q tooltip=%q", btnMax.Text, btnMax.Tooltip())
	}
	if tf.Placeholder != "Enter name..." {
		t.Fatalf("unexpected EN placeholder: %q", tf.Placeholder)
	}
	if menuBar.Menus[0].Label != "File" || menuBar.Menus[0].Items[0].Label != "Open..." {
		t.Fatalf("unexpected EN menu labels: %+v", menuBar.Menus[0])
	}
	if tabs.Tabs[0].Title != "Controls" {
		t.Fatalf("unexpected EN tab title: %q", tabs.Tabs[0].Title)
	}

	enMaxW := btnMax.MinimumSize().X
	enCurW := btnCur.Bounds().W
	enFormLblW := lblRow.MinimumSize().X
	if enMaxW <= enCurW {
		t.Fatalf("expected LocaleSizeMaxEnvelope button minWidth (%v) > LocaleSizeCurrent EN width (%v)", enMaxW, enCurW)
	}

	// Switch to Russian and verify text updates, current-size growth, and stable max-envelope width.
	mgr.SetLocale("ru")
	mgr.BeginFrame(800, 600, 0.016)

	if btnMax.Text != "Подтвердить выбранное действие" || btnMax.Tooltip() != "Подтвердить действие" {
		t.Fatalf("unexpected RU button state: text=%q tooltip=%q", btnMax.Text, btnMax.Tooltip())
	}
	if menuBar.Menus[0].Label != "Файл" || menuBar.Menus[0].Items[0].Label != "Открыть файл..." {
		t.Fatalf("unexpected RU menu labels: %+v", menuBar.Menus[0])
	}
	if tabs.Tabs[0].Title != "Виджеты и Формы" {
		t.Fatalf("unexpected RU tab title: %q", tabs.Tabs[0].Title)
	}
	ruCurW := btnCur.Bounds().W
	if ruCurW <= enCurW*1.5 {
		t.Fatalf("expected LocaleSizeCurrent button to expand in RU: en=%v ru=%v", enCurW, ruCurW)
	}
	if btnMax.MinimumSize().X != enMaxW {
		t.Fatalf("expected LocaleSizeMaxEnvelope minWidth to remain constant across locales: en=%v ru=%v", enMaxW, btnMax.MinimumSize().X)
	}
	if lblRow.MinimumSize().X != enFormLblW {
		t.Fatalf("expected localized form label minWidth to stay constant across locales: en=%v ru=%v", enFormLblW, lblRow.MinimumSize().X)
	}
}
