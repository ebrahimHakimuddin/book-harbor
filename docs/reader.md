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

## Bookmarks, highlights, and search

The bookmark button in the top bar marks the current passage (EPUB block or PDF
page). Press and hold an EPUB paragraph to highlight all of it or to choose a
passage within it. The same menu adds a note, copies the paragraph, or shares
it as a quote. To choose a passage, drag the system selection handles in a
dialog that shows the paragraph. The Contents sheet has three tabs: **Contents**, **Notes** (every
bookmark and highlight in reading order, with an **Export all** action that
shares them as plain text), and **Search**, which searches the full text of
the book. PDF search needs Android 15 or later, where the platform can extract
text from a PDF.

Annotations are saved on the device first, then synced to the reader's account
with `POST /annotations/sync` (see [`api.md`](api.md)) whenever there is a
connection. This uses the same background job as reading progress. Signing out
tries to sync them first, warns if any are still unsynced, and then clears
them from the device.

## Night reading and stats

The settings sheet shows how many minutes the reader has read today and their
streak of consecutive reading days. The **night schedule** switches to a dark
theme and dims the screen during hours the reader chooses (9 PM to 7 AM by
default), at a night brightness they can adjust. The **sleep timer** (15, 30, or
60 minutes) keeps the screen on, then closes the book, saving the reading
position, so the device can sleep.

The library shows a **Continue reading** card for the unfinished book read
most recently. A home-screen widget shows the same book and opens it directly.
The widget reads only data stored on the device, so it works offline.

## Library conveniences

Press and hold a book (or choose **About this book**) for its details: series,
tags, description, formats and sizes, progress, and actions to mark it read or
unread, add it to a list, or remove its download. For a downloaded book the
sheet lists its chapters (or takes a PDF page number); choosing one opens the
book there instead of at the saved place. Marking read or unread records an
ordinary progress event at 100% or 0%, so it syncs like reading does.

The library can sort by series (in series order) and filter by tag. The More
tab shows how much space downloads use and can remove every finished book's
download at once; a list can download all of its books. A background check
every six hours notifies the reader of new books, fulfilled requests, and
friend requests.

## Reading aids

Besides typography, the settings sheet offers hyphenation, word emphasis
(bolding the first part of each word to guide the eye), extra letter and word
spacing, an orientation lock, and paging with the volume keys. The footer
estimates the minutes left in the chapter from the reader's own pace, learned
from steady forward reading and ignoring jumps and idle time. **Read aloud**
(the headphones button) speaks the chapter from the current paragraph with the
device's text-to-speech engine, tinting and following the spoken paragraph.
**Look up a word** in a paragraph's menu hands the word to an installed
dictionary or translate app. PDFs zoom with a pinch, from 1x to 4x.

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
