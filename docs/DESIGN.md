# DESIGN (status dashboard)

Status: v1, made in step 8. The page is served by the operator itself (`--dashboard-bind-address`, off by default, D21). It is read-only, has no build step and loads nothing from the network (D22).

## Concept: the proof track

The page answers one question: can I trust the newest backup? A policy is therefore a verdict in words first, and below it a **track**: a time axis on which every run sits at its real time, drawn as the brand's check mark (docs/BRAND.md). A list shows what happened; a time-true track also shows what did not, because a missed run is an empty stretch. Four shapes carry the meaning, so nothing relies on colour alone:

| Shape | Meaning |
|---|---|
| solid teal check | backed up, restored, every check passed |
| slate check | backed up, not restored and checked yet |
| check cut in two, far piece slipped down | restored, but a check failed |
| flat bar | the backup failed, nothing was stored |
| dashed check | the next scheduled run |

Why this and not the other directions in the plan: a chain-of-custody ledger would be equal rows of text (what a table already does), and strata would copy the logo a second time. The track uses the logo's own shape and adds the one thing the logo cannot show, time.

## Tokens

| Name | Hex | Use | Contrast on Paper |
|---|---|---|---|
| Paper | `#F5F7FA` | page | |
| Ledger Ink | `#14212E` | text | 15.2:1 |
| Vouch Teal | `#0E7C73` | only "restored and checked" | 4.7:1 |
| Archive Slate | `#6B7F95` | unproven glyphs; text uses the darker step `#44566A` (7.0:1) | 3.8:1 (glyph only) |
| Mist | `#C9D3DE` | axis and ticks, decorative | 1.4:1 |
| Fracture | `#A82217` | failure | 6.7:1 |

The first five are from BRAND.md; Fracture is new. Ratios were computed with the WCAG formula. There is no warning colour: v1 has no warning state, and BRAND.md's earlier mention of one is superseded. No dark theme in v1 (`color-scheme: light`).

Type: IBM Plex Sans 400 and 600 for everything; IBM Plex Mono 400 only for identifiers (schedule, namespace, object keys, check names, commands). Scale: 13, 16, 20, 28 px and a fluid 28 to 44 px headline.

## Layout

    [logo]                                        Updated 14:02:11. Refreshes every 15 s.

    orders failed its latest restore check.        <- headline, the page's answer
    1 of 3 policies vouched for. ...

    [broken check]   orders
    Failed restore   Runs 0 2 * * * UTC in namespace default.
    check            Last backup | Last restore check | Restore took | Next run
    (one line why)   |----x----x----x----[/ x]----x-----:-- now ' next
                     Run finished 25 Oct, 02:14. Backed up and restored, but ...
                     What failed: row_count_orders
                     > Run history as a table

Policies that need attention come first. No boxes, cards or rules: spacing separates policies. Under 46 rem the verdict column stacks above the content.

## Decisions

- **One bold element**: the track. Everything else is type and space.
- **Motion**: on the first load the check marks are drawn in, oldest first (about 1 s in total). After that only a short fade when a run is selected and a 120 ms underline. `prefers-reduced-motion` turns all of it off.
- **Zero libraries.** The plan allowed two small ones (d3-scale, d3-shape). The track needs a linear scale, tick placement and straight paths, about 60 lines in `js/scale.js` and `js/glyphs.js`, so a dependency would only add weight, a vendoring step and supply-chain surface. This sandbox also could not download any.
- **Fonts** are woff2 files from the `@fontsource` packages, fetched once by `make dashboard-fonts` (versions recorded in `fonts/VERSIONS.txt`, licence in `fonts/LICENSE-IBM-Plex.txt`). Without them the page falls back to system fonts.
- **Honest data**: the response carries no connection details (host, bucket, Secret names). Restore duration and the failing check are only known for the latest run, so older runs show neither.
- **Accessibility**: each run is a button (arrow keys move along the track, Enter or Space selects), focus is visible, the detail line is a polite live region, every policy also has its history as a table, and contrast is above 4.5:1 for all text.
- **Plain-language states**: empty ("Nothing to show yet" and what to do), error ("The operator did not answer", what was last loaded, when it retries), failure without a check name (the `kubectl describe` command).
- **Removed after review**: a left-border callout around "What failed" (a common default) became plain text under a heading, and a "now" line through the glyph zone became a short tick in the label row.

## Files

`internal/dashboard/web/`: `index.html`, `app.css`, `js/{app,render,timeline,glyphs,scale,format,copy,dom}.js`, `fonts/`, and copies of `favicon.svg`, `favicon.ico`, `apple-touch-icon.png`, `logo.svg` (`make dashboard-assets`; a test fails if they drift from `assets/brand/`).

## Not checked yet

Not seen in a browser at all (the chat sandbox has none): layout, the draw-in animation, mobile width, focus rings. Open it once at 1280 and 375 px wide, tab through a track, and test with the OS "reduce motion" setting.
