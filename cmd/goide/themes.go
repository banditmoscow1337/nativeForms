package goide

import ui "github.com/banditmoscow1337/nativeForms"

// VSCodeDarkTheme returns a nativeForms Theme modeled after VS Code Dark+.
func VSCodeDarkTheme() ui.Theme {
	return ui.Theme{
		Text:           ui.RGBA(212, 212, 212, 255), // #D4D4D4
		MutedText:      ui.RGBA(133, 133, 133, 255), // #858585
		Panel:          ui.RGBA(30, 30, 30, 255),    // #1E1E1E (Editor background)
		PanelAlt:       ui.RGBA(37, 37, 38, 255),    // #252526 (SideBar / Gutter / Panel)
		Border:         ui.RGBA(60, 60, 60, 255),    // #3C3C3C
		Accent:         ui.RGBA(0, 122, 204, 255),   // #007ACC (VS Code Blue)
		AccentHover:    ui.RGBA(31, 138, 210, 255),  // #1F8AD2
		Control:        ui.RGBA(51, 51, 55, 255),    // #333337
		ControlHover:   ui.RGBA(62, 62, 66, 255),    // #3E3E42
		ControlPressed: ui.RGBA(27, 27, 28, 255),    // #1B1B1C
		Disabled:       ui.RGBA(78, 78, 82, 200),
		Danger:         ui.RGBA(241, 76, 76, 255),   // #F14C4C
		Selection:      ui.RGBA(38, 79, 120, 255),   // #264F78
		Tooltip:        ui.RGBA(37, 37, 38, 255),    // #252526
		Scrim:          ui.RGBA(0, 0, 0, 150),
		Header:         ui.RGBA(50, 50, 51, 255),    // #323233 (TitleBar / Tab header)
		FontSize:       15,
		ControlHeight:  34,
		CornerRadius:   4,
	}
}

// VSCodeLightTheme returns a nativeForms Theme modeled after VS Code Light+.
func VSCodeLightTheme() ui.Theme {
	return ui.Theme{
		Text:           ui.RGBA(51, 51, 51, 255),    // #333333
		MutedText:      ui.RGBA(110, 110, 110, 255), // #6E6E6E
		Panel:          ui.RGBA(255, 255, 255, 255), // #FFFFFF
		PanelAlt:       ui.RGBA(243, 243, 243, 255), // #F3F3F3
		Border:         ui.RGBA(206, 206, 206, 255), // #CECECE
		Accent:         ui.RGBA(0, 102, 184, 255),   // #0066B8
		AccentHover:    ui.RGBA(0, 90, 158, 255),    // #005A9E
		Control:        ui.RGBA(255, 255, 255, 255),
		ControlHover:   ui.RGBA(232, 232, 232, 255),
		ControlPressed: ui.RGBA(214, 214, 214, 255),
		Disabled:       ui.RGBA(200, 200, 200, 200),
		Danger:         ui.RGBA(229, 20, 0, 255),    // #E51400
		Selection:      ui.RGBA(173, 214, 255, 255), // #ADD6FF
		Tooltip:        ui.RGBA(243, 243, 243, 255),
		Scrim:          ui.RGBA(15, 20, 30, 110),
		Header:         ui.RGBA(221, 221, 221, 255), // #DDDDDD
		FontSize:       15,
		ControlHeight:  34,
		CornerRadius:   4,
	}
}

// VSCodeDarkSyntaxPalette returns the VS Code Dark+ syntax highlighting palette for Go.
func VSCodeDarkSyntaxPalette() map[ui.CodeStyle]ui.Color {
	return map[ui.CodeStyle]ui.Color{
		ui.CodeKeyword:  ui.RGBA(197, 134, 192, 255), // #C586C0 (Keywords / control flow)
		ui.CodeFunction: ui.RGBA(220, 220, 170, 255), // #DCDCAA (Functions & methods)
		ui.CodeType:     ui.RGBA(78, 201, 176, 255),  // #4EC9B0 (Types, structs, interfaces)
		ui.CodeString:   ui.RGBA(206, 145, 120, 255), // #CE9178 (Strings)
		ui.CodeComment:  ui.RGBA(106, 153, 85, 255),  // #6A9955 (Comments)
		ui.CodeNumber:   ui.RGBA(181, 206, 168, 255), // #B5CEA8 (Numeric literals & constants)
		ui.CodeVariable: ui.RGBA(156, 220, 254, 255), // #9CDCFE (Variables & parameters)
	}
}

// VSCodeLightSyntaxPalette returns the VS Code Light+ syntax highlighting palette for Go.
func VSCodeLightSyntaxPalette() map[ui.CodeStyle]ui.Color {
	return map[ui.CodeStyle]ui.Color{
		ui.CodeKeyword:  ui.RGBA(175, 0, 219, 255),  // #AF00DB
		ui.CodeFunction: ui.RGBA(121, 94, 38, 255),  // #795E26
		ui.CodeType:     ui.RGBA(38, 127, 153, 255), // #267F99
		ui.CodeString:   ui.RGBA(163, 21, 21, 255),  // #A31515
		ui.CodeComment:  ui.RGBA(0, 128, 0, 255),    // #008000
		ui.CodeNumber:   ui.RGBA(9, 134, 88, 255),   // #098658
		ui.CodeVariable: ui.RGBA(0, 16, 128, 255),   // #001080
	}
}
