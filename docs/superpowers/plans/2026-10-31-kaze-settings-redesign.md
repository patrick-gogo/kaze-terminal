# Settings Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** rebuild the settings page to match the approved mockup `docs/design/settings-redesign-mockup.html` (owner's yes on 2026-10-31, with the gradients removed).

**Architecture:** the page keeps its row model (`settingRow`, `ctrl`, `page`), so hit testing, keyboard focus, scrolling and every section's `act` stay as they are. What changes is how rows are drawn (in cards, under section labels), the controls' look, the menu, and a sans UI font. Updates moves out of About into its own section.

**Tech Stack:** Go 1.27, Gio v0.10.3.

**Spec:** the mockup above is the design. Decisions it fixes:
- UI text in Manrope (OFL), monospace (JetBrains Mono) only for keys, paths, commands and versions.
- Menu: "Settings" title and "Saved to config.toml as you go"; icons; Appearance, Workspaces, Claude, Shortcuts, then a "Kaze" group with Updates (badge when one is out) and About; footer with the mark, version and update state. No "Later:" line.
- Page: 24 sp title, a subtitle, an optional action on the right; uppercase section labels; rows in cards with hairline dividers; no gradients.
- Controls: 38x22 toggle, segmented with a raised selected option, stepper, buttons (primary filled accent, normal filled, ghost), status pills (ok green, warn amber, off grey, run accent).

## Global Constraints

- Every existing control id and `act` keeps working; tests that drive sections by id stay valid.
- Idle CPU stays zero: no animation.
- Works in all five themes, including the light Washi: colors come from theme tokens.
- No test reads the owner's real files.

## Review Focus

- Keyboard focus (Tab, arrows) still reaches every control after the redraw, and the focused one stays scrolled into view.
- A narrow window stacks a row's controls under its label instead of overlapping.
- Washi (light) keeps readable contrast for pills and cards.
- The Japanese 風 in About still draws (Manrope has no CJK; the collection falls back).

---

### Task 1: The UI font
`assets/fonts`: Manrope Regular, Medium, SemiBold (static instances cut from the OFL variable font) and its license. `chrome` registers them as typeface "Manrope"; `page.text` draws sans, `page.mono` draws JetBrains Mono, `page.textW` takes a weight. Test: the faces load and SemiBold measures wider than Regular.

### Task 2: Frame: menu, heading, cards, controls
Section gets `group`, `icon`, `subtitle`, `action` (header controls) and `badge`. `settingRow` gets `heading` (a section label that starts a new card), `pill`, `icon` and `mono` hint. Rows between labels draw in one card. Controls restyled. "Later" removed. Tests: no "Later" text; the menu lists the six sections in order with the Kaze group; headings start cards.

### Task 3: Updates page, About page
Move the updater block from About into an Updates section: status card (up to date / available with What's new / confirm / progress / error), the two switches, install info. About: logo lockup, version pill, a facts row, Copy build info / View on GitHub / Report an issue. Update tests move to the new section; status bar click opens Updates.

### Task 4: Appearance
Theme cards with a small terminal preview each, a live preview card, font size stepper, line spacing, cursor shape chips, blink switch.

### Task 5: Claude, Shortcuts
Claude: setup check rows with an icon tile, a mono hint and a pill (Connected / Missing / Broken / Taken) plus Fix or Replace; Alerts and Status bar cards. Commands becomes Shortcuts: grouped cards, key caps, the action id dimmed, a search field; the command line card.

### Task 6: Workspaces
Each workspace a card: initial badge, name, tab count, state pill, Run / Go to tabs, Edit; member rows with color bar, command chip, folder, wait. The editor keeps its fields in a card. "New workspace" in the header and as a dashed card.
