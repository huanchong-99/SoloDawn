# Agent Note: Ten SoloDawn theme colours tint the full interface, including warm afterglow

Status: implemented

## Problem

After the interface redesign (`6453c7d85`), changing accent colours only recoloured interactive
controls and accents on a cool slate page, leaving surfaces un-tinted. In addition, theme names
and swatches needed SoloDawn's own day-cycle naming and a warm paper variant without colliding
with semantic status colours (`--dawn` for attention, red for failures, green for success).

## Decision

The coordinator established ten SoloDawn theme colours in `Settings > Appearance` (`sd-accent` ->
`<html data-accent>`), with owner approval of the warm ivory paper tuning for `afterglow`:

1. **Ten SoloDawn theme names:** `daybreak` (default), `rain`, `pool`, `shallows`, `shade`,
   `hills`, `glow`, `afterglow`, `firstlight`, and `nightfall` — SoloDawn's own day-cycle palette,
   omitting pure red and collision-prone orange while keeping swatches visually distinct
   ([web design README](../../../../web/design/README.md) section 3.1,
   [accents.css](../../../../web/src/styles/accents.css)).
2. **Whole-interface OKLCH tinting:** Surface, hairline, and text tokens derive their hue and
   tint strength from the chosen theme colour's `--hue` and `--tint`, tinting the entire light
   paper / dark ink canvas while preserving WCAG 2.1 AA contrast across both modes (token
   definitions and exact values live in [tokens.css](../../../../web/src/styles/tokens.css) and
   [web design README](../../../../web/design/README.md) section 3.1).
3. **Warm `afterglow` theme & magenta `--dawn` shift:** Added `afterglow` (warm ivory paper
   with reduced tint under a clay accent) alongside `firstlight`; on both warm themes `--dawn`
   shifts to magenta so "needs a person" signals stay well away from the accent (about 70° of hue
   from `firstlight` and 54° from `afterglow`, keeping at least 45° of hue separation across every
   theme colour per [web design README](../../../../web/design/README.md) section 3.1 and
   [accents.css](../../../../web/src/styles/accents.css)).

## Alternatives considered

- **Accent-only colour swatches:** kept surfaces neutral cool-blue everywhere, making theme
  changes subtle and clashing against warm accents such as `afterglow` and `firstlight`.
- **Using generic colour names or vendor palette names:** avoided to keep SoloDawn's daybreak-to-
  nightfall identity consistent with the product name.

## Consequences

- Every surface and text token stays inside the sRGB gamut across all 10 themes and both
  light/dark modes, enforced by `features/web_a11y.feature` and `web/features/a11y.spec.ts`.
- Some cool/warm accents sit close in hue to green `--ok` or red `--danger` (`issues.md`), so
  status meaning continues to rely on glyphs and text rather than hue alone.

## Verification

- Implemented in commits `e0b5e3c1d` (`nine theme colours with SoloDawn's own names`) and
  `5e833244b` (`theme colours tint the whole interface, and add warm theme afterglow`), verified by
  `features/web.feature`, `features/web_a11y.feature`, `web/features/web.spec.ts`,
  `web/features/a11y.spec.ts`, and `web/src/lib/prefs.test.ts`.

## Related

- [web design README](../../../../web/design/README.md) (section 3.1)
- [accents.css](../../../../web/src/styles/accents.css)
- [tokens.css](../../../../web/src/styles/tokens.css)
- [issues.md](../../../../issues.md)
