---
name: BookHarbor
description: Calm, dependable interfaces for a private self-hosted library.
colors:
  harbor-navy: "#0f2d46"
  harbor-navy-deep: "#092238"
  sea-teal: "#2e7d7a"
  sea-teal-dark: "#23635f"
  mist-cyan: "#7fb3c3"
  sand: "#f8f6ef"
  sunrise: "#f4a261"
  paper: "#fffefb"
  ink: "#18354d"
  muted: "#60768a"
  line: "#d9e2e5"
  soft: "#eef4f4"
  danger: "#a43f3f"
typography:
  display:
    fontFamily: "Literata, Georgia, serif"
    fontWeight: 700
    lineHeight: 1
    letterSpacing: "-0.035em"
  body:
    fontFamily: "Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.88rem"
    fontWeight: 700
    lineHeight: 1.5
rounded:
  control: "9px"
  surface: "14px"
spacing:
  xs: "8px"
  sm: "12px"
  md: "18px"
  lg: "28px"
  xl: "36px"
components:
  button-primary:
    backgroundColor: "{colors.harbor-navy}"
    textColor: "{colors.paper}"
    rounded: "{rounded.control}"
    padding: "9px 16px"
    height: "42px"
  input:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "11px 13px"
---

# Design System: BookHarbor

## Overview

**Creative North Star: "The Calm Harbor"**

BookHarbor should feel like a dependable place where a personal library is
kept in order. Interfaces are spacious and quiet, with maritime color used for
orientation and state rather than decoration. Daily-use surfaces prioritize
scanability, conventional controls, and clear recovery from errors.

The serif voice belongs to the BookHarbor name and major page headings. Product
controls and dense information use the sans-serif voice. Brand character comes
from this pairing, the supplied maritime palette, and measured spacing.

**Key Characteristics:**

- Light, calm surfaces with Harbor Navy structure.
- Sea Teal reserved for selection, focus, and secondary action.
- Editorial headings paired with familiar product controls.
- Responsive layouts that become strict single columns on small screens.

## Colors

The palette moves from deep harbor blues through balanced teal and cyan, with
Sand and warm Paper carrying most interface area.

### Primary

- **Harbor Navy** (`#0f2d46`): Primary actions, headings, and structural type.
- **Deep Harbor Navy** (`#092238`): High-contrast depth and pressed states.

### Secondary

- **Sea Teal** (`#2e7d7a`): Focus, selection, and secondary actions.
- **Mist Cyan** (`#7fb3c3`): Soft supporting detail and low-emphasis accents.
- **Sunrise** (`#f4a261`): Keyboard focus only. Its rarity keeps focus obvious.

### Neutral

- **Sand** (`#f8f6ef`): Warm outer canvas and entry surfaces.
- **Paper** (`#fffefb`): Main content and form surfaces.
- **Ink** (`#18354d`): Body text.
- **Muted Harbor** (`#60768a`): Secondary copy.
- **Harbor Line** (`#d9e2e5`): Dividers and low-contrast boundaries.
- **Sea Mist** (`#eef4f4`): Selected or grouped backgrounds.

**The Navigation Rule.** Accent color communicates action or current state; it
does not decorate inactive content.

## Typography

**Display Font:** Literata (with Georgia fallback)
**Body Font:** Inter (with system sans-serif fallback)

**Character:** Literata makes the library feel literary and owned. Inter keeps
forms, navigation, and metadata direct and compact.

### Hierarchy

- **Display** (700, `2.2rem-4.25rem`, 1): One page title per surface.
- **Headline** (700, `1.45rem`, 1.2): Panel and task headings.
- **Title** (700, `0.96rem-1rem`, 1.4): Book, reader, and provider-result names.
- **Body** (400, `1rem`, 1.5): Explanatory copy, with prose kept below 70ch.
- **Label** (700, `0.88rem`, 1.5): Field and action labels.

**The Two-Voice Rule.** Serif is for identity and hierarchy, never for form
controls or dense metadata.

## Layout

Desktop administration uses a 244px navigation rail and a flexible content
canvas capped at 1320px. Task pages pair a primary list with a narrower editing
or creation panel. At 980px the rail becomes a horizontal task switcher and
two-column work areas stack. At 700px page padding contracts to 16px and all
content becomes a single column.

Spacing follows an 8, 12, 18, 28, 36px rhythm. Related labels and controls stay
tight; page regions receive visibly larger separation.

## Elevation & Depth

The system is mostly tonal. Shadows are reserved for floating task panels and
interactive book tiles, and are tinted Harbor Navy rather than black.

### Shadow Vocabulary

- **Ambient panel** (`0 14px 36px rgba(15, 45, 70, .1)`): Sticky editors and transient notices.
- **Quiet tile** (`0 7px 24px rgba(15, 45, 70, .065)`): Interactive book entries only.

**The Flat-First Rule.** Prefer spacing, dividers, and tonal surfaces; add
elevation only when it explains interaction or layering.

## Shapes

Controls use 9px corners and surfaces use 14px corners. Circular shapes are
reserved for reader avatars. Borders stay one pixel and low contrast. Small
status tags may use compact text but do not become decorative pills.

## Components

### Buttons

- **Shape:** Compact rounded rectangle (9px), minimum height 42px.
- **Primary:** Harbor Navy with warm-white text and `9px 16px` padding.
- **Hover / Focus:** Darker navy on hover; Sunrise three-pixel focus outline.
- **Secondary / Quiet:** Sea Teal fill or transparent Paper with a Harbor Line border.

### Cards / Containers

- **Corner Style:** 14px.
- **Background:** Paper for content, Sea Mist for grouped actions.
- **Shadow Strategy:** Flat by default; use the ambient panel token for elevated editors.
- **Internal Padding:** 24px for task panels, 12px for compact book entries.

### Inputs / Fields

- **Style:** Paper fill, one-pixel blue-gray stroke, 9px corners.
- **Focus:** Sea Teal stroke with a restrained teal focus field.
- **Error / Disabled:** Dark red explanatory text; disabled actions retain readable contrast.

### Navigation

Navigation uses plain-language labels and medium-weight sans-serif type. The
active destination sits on a Sea Mist field; inactive destinations remain flat.
On narrow screens, destinations form a sticky horizontal task switcher.

## Do's and Don'ts

### Do:

- **Do** use Harbor Navy for the strongest hierarchy and action.
- **Do** keep routine operations conventional, keyboard accessible, and compact.
- **Do** collapse work areas into one clear reading order on small screens.
- **Do** pair useful empty, loading, error, and success states with every workflow.

### Don't:

- **Don't** use the maritime palette as decorative gradients or glow.
- **Don't** introduce a third type voice into product UI.
- **Don't** hide account, book, or provider behavior behind storage-specific language.
- **Don't** use motion unless it communicates feedback or a state change.
