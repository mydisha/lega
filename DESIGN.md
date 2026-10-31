# lega design

lega runs in a terminal, so the design works with the user's terminal theme
instead of painting over it.

## Color

- Text uses the terminal's own foreground. Secondary text (descriptions, paths,
  hints) uses faint text, so it follows the terminal theme too.
- There is one accent color, orange. It marks only what the user acts on or
  looks for: the cursor row, a selected checkbox, and the key number on a
  screen (free space, the size selected, the size freed). The one exception
  is the wordmark on the start screen, drawn once in the accent.
  - Text on a dark background: `#f5a173`
  - Text on a light background: `#a03f15`
  - Filled bar of the focused row: `#ec6a2c`
- Red means danger (a disk over 90% full, delete-for-good prompts, errors).
  Yellow means "look before you continue" (Trash prompt, sudo warning). They
  are never used for decoration.
- The light or dark accent is picked from the terminal's reported background
  color. Dark is assumed until the terminal answers.
- No gradients, glows, background patterns or extra brand colors.

Colors live in `internal/ui/theme.go`. New screens use the `Theme` styles,
not new hex values.

## Type and layout

- Screen titles are bold, in sentence case. Labels are never uppercase.
- Sizes use decimal units (kB, MB, GB), the same as GNOME Files and Disks, and
  are right-aligned so they line up.
- Bars use `█` for the filled part and a faint `░` for the rest.
- Every screen fits 80 columns and still works at 60. Long paths are shortened
  in the middle (`/home/…/node_modules`) so the start and the name stay visible.
  Bars shrink first, then disappear, before names are cut.
- The help line with the keys for the current screen sits on the last row.
- The start screen shows the wordmark (figlet "ANSI Shadow" style) with the
  system summary beside it. On narrow screens the summary moves under it;
  when the screen is too narrow or short for the wordmark, a plain bold
  "lega" takes its place, so the menu items never scroll away.

## Icons

- Icons are Nerd Font glyphs and are off unless the user asks for them
  (`--icons` or `LEGA_ICONS=nerd`). Without a Nerd Font they would draw as
  empty boxes.
- An icon names the real thing: the app's own logo for a clean item
  (Firefox, VS Code, npm), the file kind for a file, the folder's purpose for
  a known folder (Downloads, `.git`, `node_modules`). Anything else gets the
  plain folder or file glyph, never a decorative one.
- Icons are faint, and take the accent only on the cursor row, the same as the
  name next to them.
- Each glyph takes one cell plus a space. Rows take those two cells off the
  name, so screens still fit 80 and 60 columns.

## States

Every screen handles these states:

- **Working**: a small spinner with live counts ("Scanning… 12,345 files, 4.2 GB").
- **Empty**: one plain sentence saying there is nothing to do, and what to try.
- **Error**: the error in red, with the path or command involved. Errors on a
  single item never stop the rest.
- **Done**: what changed, with the key number in the accent color, then a short
  list of what failed.

## Wording

- Plain words, short sentences, no exclamation marks.
- Say what will happen before it happens ("Files are deleted for good, not moved
  to the Trash.").
- Name the real thing: "Firefox cache", not "browser data".
