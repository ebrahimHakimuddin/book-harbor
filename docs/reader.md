# Android reader experience

The reader is offline-first and format-aware. It opens only verified local
downloads, saves position locally before attempting network work, and keeps its
primary controls usable without leaving the page.

## EPUB reading model

An EPUB chapter is one uninterrupted vertical scrolling surface. Text reflows
to the viewport and the reader does not divide a chapter into simulated pages.
Long chapters may be scrolled continuously from start to finish.

Only the current spine item is mounted in the reading surface. Images and text
inside that item load incrementally, while the next item may be parsed and
prefetched off-screen. This bounds memory use without putting loading breaks
inside a chapter.

The end of every chapter contains a deliberate transition card showing:

- completion of the current chapter;
- the next chapter's title when one exists; and
- a prominent **Next chapter** action.

Reaching the bottom never advances automatically. Activating the action swaps
in the next chapter, scrolls to its start, moves accessibility focus to its
heading, and records the new location. The last chapter instead offers a
**Finish book** action. Previous/next chapter actions also remain available in
the reader controls for non-linear navigation.

EPUB documents are untrusted content. Scripts and active content are disabled,
external network requests are blocked, archive paths are validated, and book
styles cannot escape the isolated reading surface. Publisher styling is
supported only where it remains compatible with reader overrides and
accessibility.

## PDF reading model

PDFs use a continuous vertical page list with incremental rendering and a small
page buffer around the viewport. A page number is the authoritative reopen
location. The control surface shares themes, brightness, progress display, and
navigation patterns with EPUB where the format permits them; font and reflow
controls do not pretend to change fixed-layout PDF content.

## Progress display and persistence

The normal reading chrome shows the current chapter or PDF page and overall
book percentage. EPUB may additionally show progress within the current
chapter. Readers can hide this display for a distraction-free view and reveal
it with a tap.

EPUB reopening uses an EPUB CFI captured at the first clearly visible text
position. PDF reopening uses the foremost visible page. The overall percentage
is a display summary calculated from stable publication positions across the
EPUB spine, or from page index and count for PDF. It is never used in place of
the format-specific locator.

Scrolling updates the local position at a bounded cadence and always flushes it
when the app backgrounds, the book closes, or the chapter changes. Each durable
local update creates an immutable synchronization event as defined in
[`offline-sync.md`](offline-sync.md). Reading remains fully functional while
the client or server is offline.

Changing typography can reflow an EPUB. The reader restores the same CFI after
the change and then recalculates the visible percentage, so changing font size
does not jump to an unrelated passage.

## Reader controls

The settings sheet previews changes immediately and is fully usable offline.
Initial EPUB controls include:

- theme: system, light, sepia, dark, and black;
- typeface: publisher, reader serif, reader sans serif, and an
  accessibility-focused face;
- font size;
- line height;
- paragraph spacing;
- horizontal margin;
- text alignment, including preserving the publisher default; and
- screen brightness override, with a system-brightness option.

Theme, typography, progress visibility, and brightness are stored locally as a
reader profile and apply to newly opened books. A reader may reset an individual
setting or the whole profile to defaults. Settings changes must not require a
server round trip. Cross-device preference sync is outside version 0.1; reading
progress sync is not.

The main overlay also provides table of contents, chapter/page navigation,
progress, settings, and close. Controls meet Android
touch-target and contrast requirements, respect system font scaling and reduced
motion, work with TalkBack and switch access, and never rely on color alone.

## Acceptance criteria

- A downloaded EPUB and PDF can be opened, navigated, customized, closed, and
  reopened in airplane mode.
- An EPUB chapter scrolls continuously without page snapping or an artificial
  break inside the chapter.
- The reader does not enter another chapter until the reader activates the
  next-chapter control or chooses a destination explicitly.
- The saved locator resolves to the same passage after changing EPUB typography
  or restarting the application.
- Current location and overall progress are visible on demand and may be hidden.
- Theme and typography changes apply live and persist across restarts.
- A missing server, failed request, or expired session cannot block scrolling,
  chapter navigation, or local progress persistence.
- Large chapters and long PDFs do not require the complete publication to be
  rendered into memory at once.
