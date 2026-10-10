# Stage 14.1: Russian/English text and accessibility foundation

This is the first increment of stage 14, not completion of the full multilingual
and platform-accessibility milestone. The initial target is left-to-right
Russian and English UI text. It adds no dependencies and leaves the existing
software/Vulkan command contract intact.

## Portable fonts and text positions

Configure fonts before the first text measurement, frame or `FontAtlas()` call:

```go
if err := nativeforms.SetFontData(primaryTTF); err != nil {
    return err
}
if err := nativeforms.SetFallbackFontData(cyrillicTTF, symbolsTTF); err != nil {
    return err
}
```

`SetFallbackFontData` replaces the whole fallback list. Passing no arguments
clears that list before initialization. Each call copies its buffers and either
installs every font or changes nothing. Once the atlas is initialized, both
setters return an error. The global atlas remains immutable and shared by all
managers/renderers; runtime font switching is not part of this increment.

For each atlas character, the primary font is tried first, then the configured
fallbacks. A missing or unsupported glyph advances to the next font. Without
any configured fonts, the known system-font candidates are tried in order.
An explicitly configured chain does not silently append system fonts. The last
resort is the embedded ASCII bitmap set, then `?` for missing characters.
Applications that need predictable Cyrillic coverage should supply a suitable
TTF; the ASCII fallback does not provide Russian letters.

The portable path now shares glyph positioning between measurement, drawing,
selection and caret hit testing. It applies horizontal OpenType `kern` format 0
pairs within a single face, including additive/override subtables. Unsupported
or truncated subtables are skipped. No adjustment crosses a font change.
Fonts that store kerning only in GPOS still use their unadjusted advances.

The fixed repertoire includes Russian/English, the previously supported
Latin/Greek/Cyrillic ranges, and common punctuation (`«»`, `„“”`, `‘’`, dashes,
ellipsis, `№`, `₽`, `€`, bullets and minus). Space controls no longer become
question marks: NBSP, narrow NBSP, thin space, tab (four spaces), zero-width
space and Word Joiner have appropriate blank/zero advances. Decomposed
`Е/е + U+0308` and `И/и + U+0306` use the existing `Ё/ё/Й/й` atlas cells while
retaining the original text and selection offsets. This is a targeted rendering
rule, not general normalization or combining-mark placement.

AppKit still installs Core Text, whose font fallback/shaping is independent of
these portable font setters. To inspect this portable implementation on macOS,
use the offscreen example. No native Core Text behavior is removed.

## Wrapping and editing

`WrapText` and multiline `TextField` now share line-break selection:

- Spaces, tabs, thin spaces, zero-width spaces, hyphens, slashes and em dashes
  provide common RU/EN break opportunities.
- Common opening punctuation stays with following text, and closing punctuation
  stays with preceding text. NBSP, narrow NBSP, nonbreaking hyphen and Word
  Joiner protect adjacent content. An overlong explicitly glued word may overflow.
- A long ordinary word can wrap at a grapheme boundary without an inserted
  hyphen. This is emergency wrapping, not Russian/English dictionary hyphenation.
- CRLF, CR, LF, NEL, line separator and paragraph separator produce line breaks.
  Labels/Canvas use an LF-normalized display string. Editors retain the original
  text, CRLF pairs, whitespace and rune/UTF-8 offsets for selection/copy/undo.
- Trailing breakable spaces are excluded from line fitting. `WrapText` removes
  them at inserted breaks; editor lines keep them selectable.
- Combining marks count as part of a word for word navigation/selection.

This is a documented RU/EN subset inspired by [UAX #14](https://www.unicode.org/reports/tr14/),
not full Unicode line-breaking conformance. In particular, automatic
hyphenation, discretionary soft-hyphen display, bidi layout, full quote/number
context rules and non-Latin/Cyrillic shaping remain future work. An explicit
soft hyphen is currently invisible and does not create a discretionary glyph.
The existing grapheme implementation also retains its documented limitations.

## Accessibility core

Standard labels, buttons, checkboxes, toggles, sliders, progress bars and text
fields expose default roles, names, values, states and supported actions.
`NewDialog` supplies its dialog role and title. Other components currently
appear as layout groups, with their ordinary component children.

```go
label := nativeforms.NewLabel("Имя объекта")
field := nativeforms.NewTextField("Камера", nil)
field.SetAccessibility(nativeforms.AccessibilityInfo{LabelledBy: label})
root.SetAccessibility(nativeforms.AccessibilityInfo{
    Name: "Редактор", Language: "ru",
})
```

`AccessibilityInfo` can override role, name and description, specify an
inherited language tag (`ru`/`en` initially), link a label, or hide a decorative
subtree. Language metadata does not translate strings. Label relations are
resolved only within the exposed tree; hidden/detached labels are not linked.
An explicit name takes precedence over the relation, then the widget default.
Avoid cyclic label relations. Tooltip text is the default description.

Call `manager.AccessibilitySnapshot()` on the UI owner goroutine, normally
**after `BeginFrame`**. It does not process pending tasks or perform layout.
The returned tree contains copied data and no component pointers/callbacks,
so it can be handed to a background reader. `State.AccessibilityID()` is stable
for the lifetime of that state; IDs are never reused. UI states must not be
copied after use.

Nodes include parent/children, label relation, inherited language, logical
window bounds, clipped visible bounds, focus, checked/read-only/protected
states, numeric ranges, and UTF-8 byte selection offsets. Offscreen controls
remain discoverable. Hidden subtrees are omitted; disabled state is inherited.
Password text and selection offsets are absent. With a modal open, only the
most recent modal subtree is exposed. Passive tooltips use descriptions rather
than separate interactive nodes. Noninteractive managers expose no actions.

Invoke one advertised action at a time on the UI owner goroutine:

```go
manager.PerformAccessibilityAction(field.AccessibilityID(),
    nativeforms.AccessibilityFocus, "")
manager.PerformAccessibilityAction(field.AccessibilityID(),
    nativeforms.AccessibilitySetValue, "Camera / Камера")
```

Supported actions are focus, activate, increment, decrement and set value,
according to widget type. A slider's input value uses a decimal dot independent
of UI language; NaN/infinity are rejected. Text replacement uses `Validate`,
one undo entry and `OnChange`; read-only fields reject replacement. Actions
resolve the **current** tree and reject stale, hidden, disabled and modal-blocked
targets. Calls from another goroutine must be queued with `Manager.Post`.
Snapshot text/format callbacks should be read-only, as during measurement/paint.

This API is the foundation for platform bridges. It does **not yet connect** to
VoiceOver/NSAccessibility, Windows UI Automation or Linux AT-SPI, and does not
emit native accessibility notifications. Virtual rows/cells, menu/radio/tab
items, text-range providers and custom semantic actions are not implemented.
A snapshot of the editor demo therefore is not yet a complete screen-reader UI.

## Manual acceptance on your machine

No build, test, example or runtime check was executed for this increment.
Only formatting and source/diff review were performed.

```sh
go run ./examples/text_ru_en
# Optional deterministic font chain: Latin-only primary + Cyrillic fallback.
go run ./examples/text_ru_en -font /path/to/primary.ttf -fallback-font /path/to/cyrillic.ttf
```

The example writes `text-ru-en.png` and `text-ru-en.json`. Inspect Russian
letters (including `Ё/ё/Й/й` and their decomposed equivalents), mixed-language
baselines, paired punctuation and wrapped paragraphs. With a font containing a
`kern` table, inspect `AVATAR`, `To` and `WA`. Check that the JSON has the linked
object-name label, inherited/per-label languages, and a protected password node
with no value/selection. The desktop editor now gives names to its object-name
and notes fields and assigns `ru` to its root.

In a desktop text area, paste CRLF text, move/select across wraps, edit mixed
Russian/English text, and undo. Check amounts such as `10 000`, `10 000`, and
`joined⁠word` at narrow widths. Exercise focus/activate/set-value actions from a
UI callback and repeat after hiding/removing/disabling the target or opening a
modal. Check read-only rejection, validation, callbacks and undo behavior.
These are checks for the project owner; results are not claimed here.

## Remaining parts of stage 14

1. Portable GPOS positioning and optional Latin ligatures; general combining
   marks (including Russian stress), cluster/run fallback and dynamic glyph
   storage beyond the fixed atlas.
2. Complete semantic models for collection items, custom controls and text
   ranges, followed by platform bridges and native change notifications.
3. Full Unicode line-break/property data and, when languages require them,
   bidi and the remaining OpenType shaping features.

The first two items remain useful for a Russian/English product. Supporting
additional writing systems is not a prerequisite for implementing them.

Font-table reference: [OpenType `kern`](https://learn.microsoft.com/en-us/typography/opentype/spec/kern).
