<!--
Adapted from the "soft-skill" skill in Leonxlnx/taste-skill
Source: https://github.com/Leonxlnx/taste-skill
License: MIT. See the SoloDawn LICENSE file for full attribution.
Changes: condensed and adapted for automated prompt injection (2026-07-04).
-->

# Soft Premium UI and Motion Choreography

Engineer agency-level digital experiences worth a $150k build, not just websites. The output must exude haptic depth, cinematic spatial rhythm, obsessive micro-interactions, and flawless fluid motion in the elite Apple-esque / Linear-tier design language. Never generate the exact same layout or aesthetic twice in a row: dynamically combine premium layout archetypes and texture profiles per project.

## Strict anti-patterns — the design instantly fails if any appear

- Banned fonts: Inter, Roboto, Arial, Open Sans, Helvetica. Assume premium faces like `Geist`, `Clash Display`, `PP Editorial New`, or `Plus Jakarta Sans` are available.
- Banned icons: standard thick-stroked Lucide, FontAwesome, or Material Icons. Use only ultra-light, precise lines (Phosphor Light, Remix Line).
- Banned borders/shadows: generic 1px solid gray borders; harsh dark drop shadows (`shadow-md`, `rgba(0,0,0,0.3)`).
- Banned layouts: edge-to-edge sticky navbars glued to the top; symmetrical, boring 3-column Bootstrap-style grids without massive whitespace gaps.
- Banned motion: standard `linear` or `ease-in-out` transitions; instant state changes without interpolation.

## Creative variance — pick ONE vibe and ONE layout archetype per project

Vibe and texture:
1. Ethereal Glass (SaaS / AI / tech): deepest OLED black (`#050505`); radial mesh gradients (subtle glowing purple/emerald orbs) in the background; vantablack cards with heavy `backdrop-blur-2xl` and pure white/10 hairlines; wide geometric grotesk typography.
2. Editorial Luxury (lifestyle / real estate / agency): warm creams (`#FDFBF7`), muted sage, or deep espresso tones; high-contrast variable serif for massive headings; subtle CSS noise/film-grain overlay (`opacity-[0.03]`) for a physical paper feel.
3. Soft Structuralism (consumer / health / portfolio): silver-grey or completely white backgrounds; massive bold grotesk typography; airy, floating components with unbelievably soft, highly diffused ambient shadows.

Layout:
1. Asymmetrical Bento: masonry-like CSS Grid of varying card sizes (`col-span-8 row-span-2` next to stacked `col-span-4`). Mobile: single-column stack (`grid-cols-1`, `gap-6`); all `col-span` overrides reset to `col-span-1`.
2. Z-Axis Cascade: elements stacked like physical cards, slightly overlapping at varying depths, some rotated `-2deg` or `3deg`. Below `768px`: remove all rotations and negative-margin overlaps and stack vertically — overlaps cause touch-target conflicts.
3. Editorial Split: massive typography on the left half (`w-1/2`); interactive, scrollable horizontal image pills or staggered cards on the right. Mobile: full-width vertical stack, typography on top, horizontal scroll preserved where needed.

Universal mobile override: below `768px`, every asymmetric layout aggressively falls back to `w-full`, `px-4`, `py-8`. Never use `h-screen` for full-height sections — always `min-h-[100dvh]` to prevent iOS Safari viewport jumping.

## Haptic micro-aesthetics

- The Double-Bezel (nested architecture): never place a premium card, image, or container flat on the background; it must read like machined hardware (a glass plate sitting in an aluminum tray). Outer shell: subtle background (`bg-black/5` or `bg-white/5`), hairline border (`ring-1 ring-black/5` or `border border-white/10`), padding `p-1.5` or `p-2`, large radius (`rounded-[2rem]`). Inner core: its own distinct background, an inner highlight (`shadow-[inset_0_1px_1px_rgba(255,255,255,0.15)]`), and a mathematically smaller radius (`rounded-[calc(2rem-0.375rem)]`) for concentric curves.
- Nested CTA ("island" buttons): primary buttons are fully rounded pills (`rounded-full`, `px-6 py-3`). A trailing arrow never sits naked next to the text — nest it inside its own circular wrapper (`w-8 h-8 rounded-full bg-black/5 dark:bg-white/10 flex items-center justify-center`) flush with the button's right inner padding.
- Spatial rhythm: double your standard padding — `py-24` to `py-40` per section; let the design breathe heavily. Precede major H1/H2s with a microscopic pill eyebrow (`rounded-full px-3 py-1 text-[10px] uppercase tracking-[0.2em] font-medium`).

## Motion choreography

All motion must simulate real-world mass and spring physics via custom cubic-beziers (e.g. `transition-all duration-700 ease-[cubic-bezier(0.32,0.72,0,1)]`).

- Fluid island nav: the navbar floats as a detached glass pill (`mt-6`, `mx-auto`, `w-max`, `rounded-full`). On click, the hamburger lines fluidly rotate and translate into a perfect X (`rotate-45` / `-rotate-45` with absolute positioning) — never just disappear. The menu opens as a massive screen-filling overlay with heavy glass (`backdrop-blur-3xl bg-black/80` or `bg-white/80`); links fade in and slide up (`translate-y-12 opacity-0` to `translate-y-0 opacity-100`) with staggered delays (`delay-100`, `delay-150`, `delay-200`).
- Magnetic button physics: on hover, never just change the background color. Scale the button down on press (`active:scale-[0.98]`); translate the nested icon circle diagonally (`group-hover:translate-x-1 group-hover:-translate-y-[1px]`) and scale it up slightly (`scale-105`) for internal kinetic tension.
- Scroll interpolation: elements never appear statically. As they enter the viewport, execute a gentle, heavy fade-up (`translate-y-16 blur-md opacity-0` resolving to `translate-y-0 blur-0 opacity-100` over 800ms+). Use `IntersectionObserver` or Framer Motion `whileInView`; never `window.addEventListener('scroll')`.

## Performance guardrails

- Animate exclusively via `transform` and `opacity`; never `top`, `left`, `width`, or `height`. Use `will-change: transform` sparingly, only on actively animating elements.
- Apply `backdrop-blur` only to fixed or sticky elements (navbars, overlays); never to scrolling containers or large content areas.
- Grain/noise overlays only on fixed, `pointer-events-none` pseudo-elements (`position: fixed; inset: 0`); never on scrolling containers.
- Z-index discipline: no arbitrary `z-50` or `z-[9999]`; reserve z-indexes strictly for systemic layers (sticky nav, modals, overlays, tooltips).

## Pre-delivery checklist

- No banned fonts, icons, borders, shadows, layouts, or motion patterns are present.
- A vibe archetype and a layout archetype were consciously selected and applied.
- All major cards and containers use the Double-Bezel nested architecture; CTAs use the button-in-button pattern where applicable.
- Section padding is at minimum `py-24`; all transitions use custom cubic-bezier curves; scroll entry animations are present.
- The layout collapses gracefully below `768px` to single-column `w-full px-4`; animations use only `transform`/`opacity`; `backdrop-blur` only on fixed/sticky elements.
- The overall impression reads as an agency build, not a template with nice fonts.
