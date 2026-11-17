# Design: the Matrix interface

The app should feel like a terminal that happens to control a computer: black screen, green phosphor, monospace type, glyphs falling behind everything. The look is original work inspired by the "digital rain" genre; it uses no film assets, logos, names or quotes.

Rule that outranks every other rule in this file: **animation never delays input**. A tap sends its message on touch-down or touch-up as appropriate; the animation plays alongside.

## 1. Colour tokens

| Token | Hex | Use |
|---|---|---|
| `void` | `#000000` | Screen background (true black, OLED-friendly) |
| `surface` | `#050A05` | Cards, sheets |
| `surfaceRaised` | `#0A140A` | Pressed or selected surfaces |
| `phosphorDim` | `#003B00` | Borders at rest, rain tail, disabled |
| `phosphor` | `#008F11` | Secondary text, icons at rest |
| `phosphorBright` | `#00FF41` | Primary text, active elements |
| `head` | `#D5FFD9` | Rain head glyph, highlights, focus flash |
| `warn` | `#FFB000` | Reconnecting, caution |
| `danger` | `#FF2A2A` | Power actions, errors, revoked |
| `scrim` | `#000000` at 70 % | Behind dialogs |

Accent variants swap the three phosphor tones and `head`: **amber** (`#3B2A00`, `#8F6A00`, `#FFB000`), **cyan** (`#00303B`, `#00788F`, `#00E5FF`), **red** (`#3B0000`, `#8F1111`, `#FF4141`). Everything is derived from tokens; no hard-coded colours in feature code.

Glow: bright elements get a soft outer glow (blur 8 dp, the element's own colour at 35 % alpha). Glow is drawn, not a shadow, and is dropped under Reduced motion to save GPU.

## 2. Typography

One family: **JetBrains Mono** (SIL OFL), bundled. Rain glyphs use **Noto Sans Mono CJK JP** subset (half-width katakana) plus digits and a few symbols.

| Style | Size / weight | Use |
|---|---|---|
| `display` | 28 sp / Bold, letter-spacing 2 sp, uppercase | Screen titles |
| `title` | 18 sp / Medium, uppercase | Section headers, button labels |
| `body` | 15 sp / Regular | Lists, descriptions |
| `mono` | 13 sp / Regular | Status line, paths, logs |
| `micro` | 11 sp / Regular, uppercase | Hints, key captions |

Text conventions: prompts start with `> `, status uses `::` separators (`> DESKTOP-JOSE :: online :: 3 ms`), section headers look like `[ MEDIA ]`, a blinking block cursor `█` ends the active line.

## 3. Shape and layout

- Corners: **cut corners** (chamfer 6 dp on top-left and bottom-right), never rounded. Implemented as one `CutCornerShape` token.
- Borders: 1 dp `phosphorDim` at rest, `phosphorBright` when pressed or focused. Fills are near-black; the border carries the shape.
- Grid: 8 dp base, 16 dp screen padding, touch targets at least 56 dp.
- Layers, back to front: `void` -> rain -> vignette -> content -> scanlines -> transient effects (glitch, flashes).

## 4. Components (`core:design`)

| Component | Description |
|---|---|
| `MatrixScaffold` | Draws the layer stack; hosts status line and command bar |
| `StatusLine` | Top line with host, state and latency; text re-decodes when state changes |
| `CommandBar` | Bottom navigation as bracketed labels `[PAD] [KEYS] [MEDIA] [REMOTES] [MORE]`; active item is bright with a cursor underline |
| `TerminalButton` | Cut-corner bordered button, label or icon; variants default / primary / danger |
| `KeyCap` | Small square button for keyboard keys, with latch state for modifiers |
| `HoldRing` | Button that needs a hold; a ring of glyphs fills around it |
| `GlyphSlider` | Slider drawn as a row of block characters `▮▮▮▮▯▯▯` |
| `VolumeRing` | Circular volume control made of segmented arcs |
| `TerminalList` | List rows prefixed with an index `00 01 02`, selection inverts the row |
| `DecodeText` | Text that resolves from random glyphs (section 5.2) |
| `PromptField` | Text field styled as `> _` with block cursor |
| `HexDialog` | Dialog with a title bar like `[ CONFIRM ]` and a glitch-in entrance |
| `SignalDot` | Three-state indicator with a pulse when online |

## 5. Animations

All durations scale with the intensity setting: **Full** (as listed), **Reduced** (rain static or off, no glitch, no scanlines, decode shortened to 120 ms, simple fades), **Off** (no motion beyond state changes). The system "Remove animations" setting forces **Off**.

### 5.1 Digital rain (ambient background)

- Columns of glyphs falling at different speeds behind all content. Column width = glyph width at 14 sp; each column has speed (4–14 rows/s), length (8–28 glyphs) and a random start delay.
- Head glyph in `head` colour, the next two in `phosphorBright`, the tail fading through `phosphor` to `phosphorDim` to transparent.
- Glyphs in a stream mutate at random (about 3 % of visible glyphs per tick).
- Rendering: one `Canvas`, glyphs pre-rasterised into an atlas bitmap once per size and colour ramp, drawn with `drawBitmap` slices. No per-frame text layout, no per-frame allocation.
- Ticks at **30 fps**, independent of UI frame rate; paused when the screen is not visible or the device is in battery saver.
- Opacity by context: 100 % on Boot and Hosts, 35 % behind lists, **15 %** behind the touchpad and remotes so controls stay readable.
- Density setting: Sparse / Normal / Dense (every 3rd, 2nd, or every column active).
- Reactive touches: connecting speeds the rain up briefly; on disconnect the columns freeze, then drain downward.

### 5.2 Decode text

Text appears as random glyphs that lock into the final characters left to right.

- 22 ms per character lock, each unresolved character re-rolled every 40 ms, total capped at 450 ms regardless of length.
- Used for screen titles, the status line on state change, host names appearing in discovery, dialog titles.
- Never used for body text the user needs to read immediately, and never replayed on recomposition, only when the content changes.

### 5.3 Glitch transition

Screen changes and dialog entrances: 160 ms, three steps.

1. Horizontal slices of the outgoing screen shift by ±4–12 dp, with a 2 dp red/cyan channel split (frames 1–3).
2. One frame of `head`-coloured scanline flash.
3. Incoming screen settles with a 60 ms fade while titles decode.

Implemented with `graphicsLayer` and a `RuntimeShader` (AGSL) on Android 13+; on older versions a simpler slice-offset drawn on Canvas with no channel split.

### 5.4 CRT overlay

- Scanlines: 1 dp dark line every 3 dp at 8 % alpha, static.
- Vignette: radial darkening to 40 % at the corners.
- Flicker: brightness varies ±1.5 % at random intervals. Off by default; opt-in under Full.

### 5.5 Touchpad feedback

- **Trail**: the finger leaves a short trail of fading glyphs (last 12 positions, 300 ms decay).
- **Tap**: a ring of glyphs expands from the touch point (120 ms) and fades; right-click uses a square ring, middle-click a diamond.
- **Scroll**: thin vertical chevron streams move in the scroll direction along the edge.
- **Drag active**: surface border turns bright and pulses slowly while the button is held.
- All drawn on the same Canvas as the surface; trail data is a fixed-size ring buffer.

### 5.6 Controls

- **Button press**: border snaps to bright at touch-down (0 ms), fill inverts for 80 ms, label jitters one glyph for 60 ms, light haptic tick.
- **Modifier latch**: key cap stays inverted with a blinking corner mark.
- **HoldRing**: glyph ring fills over 900 ms; haptic ticks grow closer together; release early and the ring drains; completion triggers a bright flash and a stronger haptic.
- **GlyphSlider / VolumeRing**: segments light up one by one with a 12 ms stagger; value text re-decodes on release.
- **Block cursor**: 530 ms on, 530 ms off.

### 5.7 State moments

| Moment | Animation |
|---|---|
| App start | Rain begins from the top, app name decodes, cursor blinks, then the Hosts list types in row by row (40 ms stagger). Max 900 ms total |
| Discovery | Each found host appears as a decoding line; `SignalDot` pulses |
| Pairing scan | Viewfinder with corner brackets that "seek" inward; on success the brackets lock, flash, and the verification phrase decodes word by word |
| Connecting | Status line shows a rolling hex stream; rain speeds up |
| Connected | One full-width bright sweep from top to bottom (220 ms), single medium haptic |
| Reconnecting | Status line turns `warn`, content dims to 60 %, rain slows |
| Connection lost | One glitch burst, rain freezes then drains, status line turns `danger` |
| PC locked | Content dims; a `[ LOCKED ]` tag decodes in the status line |
| Power action sent | Screen "powers down": content collapses to a horizontal line, then a dot (280 ms), then returns to Hosts |

### 5.8 Haptics

Light tick on taps and key presses, double tick on right-click, ramp during hold, medium pulse on connect, long low pulse on disconnect. Strength is a setting, including Off.

## 6. Performance budget

- Touchpad interaction stays at the display's refresh rate with all effects at Full on a mid-range device; the rain is the first thing to be throttled if frame time exceeds budget.
- Rain: no more than 2 ms per tick. Overlays are static bitmaps drawn once and cached.
- Effects never trigger recomposition of content; they live in draw-phase modifiers and read animation state inside draw lambdas only.
- `Keep screen on` dims the screen to a low-brightness "standby rain" after 60 s without touch.

## 7. Accessibility

- Contrast: `phosphorBright` on `void` far exceeds 7:1. Secondary text uses `phosphor` only at 15 sp or larger; smaller hints use `phosphorBright` at reduced weight instead.
- Every control has a content description; decode and glitch effects are skipped for TalkBack users so text is announced once.
- Font scaling is respected up to 1.5×; layouts reflow instead of truncating.
- State is never shown by colour alone: status text and icon shape change as well.

## 8. Daemon admin page

Same tokens, same font, a lighter canvas rain behind the QR code, decode effect on headings. The QR code is rendered dark-on-light inside a bright bordered box so cameras read it reliably; the green look stops at its border.

## 9. Icon and name

App icon: a single bright glyph column forming a stylised "K" on black, with one head glyph in `head` colour. Adaptive icon with a monochrome layer for themed icons.
