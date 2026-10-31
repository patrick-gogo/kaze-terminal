# Kaze lander redesign

Approved by the owner on November 1, 2026, after reviewing the clickable mockups.
Implementation follows the approved design below.

## Approved design

Keep Kaze's existing dark palette, lavender accent, fonts and real product
captures. Keep the logo and byline in the header. Use the established tagline,
“Light, fast, and out of the way.”, as the hero headline.

Supporting copy: “A lightweight terminal for Claude Code and Codex. Keep
sessions in view, see when an agent needs you, and keep your servers close.”

On desktop, put the copy and platform downloads beside a large product capture.
On mobile, stack them with the product capture visible in the first viewport.
Retain the full-size image link, honest alt text, all eight screenshot labels,
platform-specific downloads and installation instructions.

Rotate the eight real captures every 2.5 seconds with a brief opacity transition.
No added arrows, counter or pause/play bar. Manual selection stops rotation.
Hover, keyboard focus, offscreen content and hidden pages suspend rotation;
reduced-motion preferences start the gallery paused and skip transitions.
Check eligibility again when the timer fires, as well as on interaction events.

Show only three recent releases on the lander. Link to `docs/changelog.html`
for the full history, preserving all eight existing entries and their notes.
The archive links back to the lander and to GitHub releases. Update release
instructions to maintain the full archive and the three-entry summary together.

## Implementation

Production files are `docs/index.html`, `docs/changelog.html`,
`docs/site-assets/site.css` and `docs/site-assets/site.js`. No preview banners,
base URLs or mockup routes appear in production. Shared styles and scripts
contain the implementation; the archive needs no JavaScript.

The removed download-count UI no longer triggers an unused GitHub API request.
Capture notes describe native macOS captures without assigning version 0.15 to
the older subagent captures. No product screenshots were generated or retouched.
No new dependencies, fabricated metrics or unbuilt agent-team claims were added.

## Verification

Check both pages at 1280 px and in a 390 px iframe. Verify no horizontal page
scroll, all eight images, automatic rotation, focus/manual/reduced-motion pauses,
full-size screenshot links, platform selection and internal navigation. Preserve
all original release notes. Run the JavaScript syntax and whitespace checks.
The static design detector identifies additional pre-existing small-text issues
in lower sections outside the approved hero and archive scope.

This site-only change does not bump the app version or need a release.
