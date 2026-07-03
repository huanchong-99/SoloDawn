<!--
Adapted from the "impeccable" skill in pbakaus/impeccable
Source: https://github.com/pbakaus/impeccable
License: Apache-2.0. See the SoloDawn LICENSE file for full attribution.
Changes: condensed and adapted for automated prompt injection (2026-07-04).
-->

# Impeccable Design Language

Produce ready-to-ship, production-grade interfaces, not prototypes or starting points. Do not stop until the implementation is complete: beautiful, responsive, fast, precise, bug-free, on brand. Take attention to detail seriously — the rules below are non-negotiable.

## Color

- Verify contrast. Body text must hit at least 4.5:1 against its background; large text (18px+, or bold 14px+) needs at least 3:1. Placeholder text needs the same 4.5:1, not the muted-gray default. The most common failure is muted gray body text on a tinted near-white: if contrast is even close, bump the body color toward the ink end of the ramp. Light gray "for elegance" is the single biggest reason AI designs feel hard to read.
- Gray text on a colored background looks washed out. Use a darker shade of the background's own hue, or a transparency of the text color.

## Typography

- Cap body line length at 65-75ch.
- Never pair fonts that are similar but not identical (two geometric sans-serifs, two humanist sans-serifs). Pair on a contrast axis (serif + sans, geometric + humanist) or use one family in multiple weights.
- Hero/display heading ceiling: clamp() max <= 6rem (~96px). Above that the page is shouting, not designing.
- Display heading letter-spacing floor: >= -0.04em. Anything tighter and letters touch — cramped, not "designed".
- Use `text-wrap: balance` on h1-h3 for even line lengths; `text-wrap: pretty` on long prose to reduce orphans.

## Layout

- Vary spacing for rhythm.
- Cards are the lazy answer. Use them only when they are truly the best affordance. Nested cards are always wrong.
- Flexbox for 1D, Grid for 2D; do not default to Grid when `flex-wrap` would be simpler. Responsive grids without breakpoints: `repeat(auto-fit, minmax(280px, 1fr))`.
- Build a semantic z-index scale (dropdown, sticky, modal-backdrop, modal, toast, tooltip). Never arbitrary values like 999 or 9999.

## Motion

- Motion is part of the build, not an afterthought. Do not animate CSS layout properties unless truly needed.
- Ease out with exponential curves (ease-out-quart / quint / expo). No bounce, no elastic. Use libraries (motion, gsap, anime.js, lenis) for advanced needs.
- Reduced motion is not optional: every animation needs a `@media (prefers-reduced-motion: reduce)` alternative — typically a crossfade or instant transition.
- Staggering items within one list is legitimate. The tell is the uniform reflex (one identical entrance applied to every section), not motion itself; each reveal should fit what it reveals. Never ship a page with no motion at all for fear of this.
- Reveal animations must enhance an already-visible default. Never gate content visibility on a class-triggered transition: transitions pause in hidden tabs and headless renderers, and the section ships blank.
- Premium motion materials go beyond transform/opacity: blur, backdrop-filter, clip-path, mask, and shadow/glow belong in the palette when they materially improve the effect and stay smooth.

## Interaction

- Dropdowns rendered with `position: absolute` inside an `overflow: hidden` or `overflow: auto` container get clipped. Use the native `<dialog>` / popover API, `position: fixed`, or a portal to escape the stacking context.

## Color and theme for new projects

- Use OKLCH throughout.
- The cream/sand/beige body background is the saturated AI default. The whole warm-neutral band (OKLCH L 0.84-0.97, C < 0.06, hue 40-100) reads as cream/paper/parchment regardless of name, and token names like `--paper`, `--cream`, `--sand`, `--bone`, `--linen`, `--parchment`, `--ivory` are tells in themselves. Do not translate "warm / editorial / traditional" briefs into a near-white warm-tinted background. Pick instead: (a) a saturated brand color as the body (terracotta, oxblood, deep ochre, near-black); (b) a true off-white at chroma 0, or chroma toward the brand's own hue; or (c) a darker mid-tone tinted neutral that is clearly the brand's own. Warmth is carried by accent + typography + imagery, not by the body background.
- Tinted neutrals: add 0.005-0.015 chroma toward the brand's hue. Never default-tint warm or cool "because the brand feels that way".
- Dark vs. light is never a default — not dark "because tools look cool dark", not light "to be safe". Before choosing, write one sentence of physical scene: who uses this, where, under what ambient light, in what mood. If the sentence does not force the answer, add detail until it does.
- Pick a color strategy before picking colors: Restrained (tinted neutrals + one accent <= 10%; product default), Committed (one saturated color carries 30-60% of the surface; identity-driven pages), Full palette (3-4 named roles, each used deliberately), Drenched (the surface IS the color; brand heroes, campaign pages).

## Absolute bans

If about to write any of these, rewrite the element with different structure:

- Side-stripe borders: `border-left` or `border-right` greater than 1px as a colored accent on cards, list items, callouts, or alerts. Never intentional. Rewrite with full borders, background tints, leading numbers/icons, or nothing.
- Gradient text: `background-clip: text` combined with a gradient. Use a single solid color; emphasis via weight or size.
- Glassmorphism as default: blurs and glass cards used decoratively. Rare and purposeful, or nothing.
- The hero-metric template: big number, small label, supporting stats, gradient accent. SaaS cliche.
- Identical card grids: same-sized cards with icon + heading + text, repeated endlessly.
- A tiny uppercase tracked eyebrow above every section: one named kicker as a deliberate brand system is voice; an eyebrow on every section is AI grammar. Choose a different cadence.
- Numbered section markers (01 / 02 / 03) as default scaffolding: numbers earn their place only when the section actually IS a sequence and the order carries information the reader needs.
- Text that overflows its container: test heading copy at every breakpoint; if it overflows, reduce the clamp max or rewrite the copy. The viewport is part of the design.

## The AI slop test

If someone could look at this interface and say "AI made that" without doubt, it has failed. Run the category-reflex check at two altitudes:

- First-order: if someone could guess the theme + palette from the category alone, it is the first training-data reflex. Rework the scene sentence and color strategy until the answer is not obvious from the domain.
- Second-order: if someone could guess the aesthetic family from category-plus-anti-references ("AI workflow tool that's not SaaS-cream, so editorial-typographic"; "fintech that's not navy-and-gold, so terminal-native dark mode"), it is the trap one tier deeper. Rework until both answers are non-obvious.
