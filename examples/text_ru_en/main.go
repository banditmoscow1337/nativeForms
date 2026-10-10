package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/software"
)

func main() {
	font := flag.String("font", "", "primary quadratic TrueType font path (optional)")
	fallback := flag.String("fallback-font", "", "fallback TrueType font path (optional)")
	output := flag.String("out", "text-ru-en.png", "PNG output path")
	semantics := flag.String("semantics", "text-ru-en.json", "semantic snapshot output path")
	flag.Parse()
	if err := run(*font, *fallback, *output, *semantics); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(primaryPath, fallbackPath, output, semantics string) error {
	fonts := ui.NewFontContext()
	if primaryPath != "" {
		data, err := os.ReadFile(primaryPath)
		if err != nil {
			return err
		}
		if err = fonts.AddFont("application", 400, false, data); err != nil {
			return err
		}
	}
	if fallbackPath != "" {
		data, err := os.ReadFile(fallbackPath)
		if err != nil {
			return err
		}
		if err = fonts.AddFont("fallback", 400, false, data); err != nil {
			return err
		}
	}
	fonts.AddSystemFallbacks()
	root := ui.NewStack(ui.Vertical, 12)
	root.Padding = ui.All(20)
	root.Background = ui.DefaultTheme().Panel
	root.SetAccessibility(ui.AccessibilityInfo{Name: "Русский и английский", Language: "ru"})
	title := ui.NewLabel("Hello, мир! Ёжик, йод — AVATAR, To, WA")
	title.FontSize = 26
	root.Add(title, ui.NewLabel("Е\u0308 е\u0308 И\u0306 и\u0306 / Ё ё Й й; «кавычки», “quotes”, № 14, 10\u00a0000 ₽"))
	columns := ui.NewStack(ui.Horizontal, 16)
	for _, specimen := range []struct{ language, text string }{
		{"ru", "Русский: «перенос строк», слово-дефис и неразрывные 10\u00a0000 ₽. Оченьдлинноесловобезпробелов.\r\nНовая строка после CRLF."},
		{"en", "English: (wrapped text), a hyphenated-word and joined\u2060word. AReallyLongWordWithoutSpaces.\nA new line after LF."},
	} {
		label := ui.NewLabel(specimen.text)
		label.Wrap = true
		label.SetFlex(1)
		label.SetAccessibility(ui.AccessibilityInfo{Language: specimen.language})
		columns.Add(label)
	}
	root.Add(columns)
	label := ui.NewLabel("Имя объекта / Object name")
	field := ui.NewTextField("Камера / Camera", nil)
	field.SetAccessibility(ui.AccessibilityInfo{LabelledBy: label})
	root.Add(label, field, ui.NewCheckbox("Виден / Visible", true, nil))
	password := ui.NewTextField("private value", nil)
	password.Password = true
	password.SetAccessibility(ui.AccessibilityInfo{Name: "Пароль / Password"})
	root.Add(password)
	notes := ui.NewTextArea("Заметки: слова переносятся целиком.\r\nNotes: select, copy and edit both languages.", nil)
	notes.SetAccessibility(ui.AccessibilityInfo{Name: "Заметки / Notes"})
	root.Add(notes)

	manager := ui.New()
	manager.SetFontContext(fonts)
	manager.SetInteractive(true)
	manager.SetRoot(root)
	manager.BeginFrame(840, 700, 0)
	dst := image.NewRGBA(image.Rect(0, 0, 840, 700))
	if err := software.New().Render(manager.Frame(), dst); err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, dst)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	data, err := json.MarshalIndent(manager.AccessibilitySnapshot(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(semantics, append(data, '\n'), 0644)
}
