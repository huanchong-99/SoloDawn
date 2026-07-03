<!--
Adapted from the "minimalist-ui" skill in Leonxlnx/taste-skill
Source: https://github.com/Leonxlnx/taste-skill
License: MIT. See the SoloDawn LICENSE file for full attribution.
Changes: condensed and adapted for automated prompt injection (2026-07-04).
-->

# Premium Utilitarian Minimalism and Editorial UI

Build highly refined, ultra-minimalist, document-style web interfaces analogous to top-tier workspace platforms. Enforce a high-contrast warm monochrome palette, a bespoke typographic hierarchy, meticulous structural macro-whitespace, bento-grid layouts, and an ultra-flat component architecture with deliberate muted pastel accents. Actively reject standard generic SaaS design trends.

## Absolute negative constraints (banned elements)

- DO NOT use the "Inter", "Roboto", or "Open Sans" typefaces.
- DO NOT use generic thin-line icon libraries like "Lucide", "Feather", or standard "Heroicons".
- DO NOT use Tailwind's default heavy drop shadows (`shadow-md`, `shadow-lg`, `shadow-xl`). Shadows must be practically non-existent or heavily customized to be ultra-diffuse and low opacity (< 0.05).
- DO NOT use primary colored backgrounds for large elements or sections (no bright blue, green, or red hero sections).
- DO NOT use gradients, neon colors, or 3D glassmorphism (beyond subtle navbar blurs).
- DO NOT use `rounded-full` (pill shapes) for large containers, cards, or primary buttons.
- DO NOT use emojis anywhere in code, markup, text content, headings, or alt text. Replace with proper icons or clean SVG primitives.
- DO NOT use generic placeholder names like "John Doe", "Acme Corp", or "Lorem Ipsum". Use realistic, contextual content.
- DO NOT use AI copywriting cliches: "Elevate", "Seamless", "Unleash", "Next-Gen", "Game-changer", "Delve". Write plain, specific language.

## Typographic architecture

Rely on extreme typographic contrast and premium font selection to establish an editorial feel.

- Primary sans-serif (body, UI, buttons): `font-family: 'SF Pro Display', 'Geist Sans', 'Helvetica Neue', 'Switzer', sans-serif`.
- Editorial serif (hero headings and quotes): `font-family: 'Lyon Text', 'Newsreader', 'Playfair Display', 'Instrument Serif', serif`, with tight tracking (`letter-spacing: -0.02em` to `-0.04em`) and tight line-height (`1.1`).
- Monospace (code, keystrokes, metadata): `font-family: 'Geist Mono', 'SF Mono', 'JetBrains Mono', monospace`.
- Body text must never be absolute black (`#000000`). Use off-black/charcoal (`#111111` or `#2F3437`) with a generous `line-height` of `1.6`. Secondary text: muted gray `#787774`.

## Color palette (warm monochrome plus spot pastels)

Treat color as a scarce resource, used only for semantic meaning or subtle accents.

- Canvas / background: pure white `#FFFFFF` or warm bone/off-white `#F7F6F3` / `#FBFBFA`.
- Primary surface (cards): `#FFFFFF` or `#F9F9F8`.
- Structural borders / dividers: ultra-light gray `#EAEAEA` or `rgba(0,0,0,0.06)`.
- Accents: exclusively highly desaturated, washed-out pastels for tags, inline code backgrounds, or subtle icon backgrounds:
  - Pale red `#FDEBEC` (text `#9F2F2D`)
  - Pale blue `#E1F3FE` (text `#1F6C9F`)
  - Pale green `#EDF3EC` (text `#346538`)
  - Pale yellow `#FBF3DB` (text `#956400`)

## Component specifications

- Bento feature grids: asymmetrical CSS Grid layouts. Cards must have exactly `border: 1px solid #EAEAEA`, crisp border-radius (`8px` or `12px` maximum), and generous internal padding (`24px` to `40px`).
- Primary CTA buttons: solid background `#111111`, text `#FFFFFF`, slight border-radius (`4px` to `6px`), no box-shadow. Hover: a subtle shift to `#333333` or a micro-scale `transform: scale(0.98)`.
- Tags and status badges: pill-shaped (`border-radius: 9999px`), very small typography (`text-xs`), uppercase with wide tracking (`letter-spacing: 0.05em`), backgrounds drawn from the muted pastels above.
- Accordions (FAQ): strip all container boxes; separate items only with `border-bottom: 1px solid #EAEAEA`; use clean, sharp `+` and `-` icons for the toggle state.
- Keystroke micro-UIs: render shortcuts as physical keys using `<kbd>` tags: `border: 1px solid #EAEAEA`, `border-radius: 4px`, `background: #F7F6F3`, monospace font.
- Faux-OS window chrome: when mocking up software, wrap it in a minimalist container with a white top bar containing three small, light gray circles (macOS-style window controls).

## Iconography and imagery

- System icons: Phosphor Icons (Bold or Fill weights) or Radix UI Icons for a technical, slightly thicker-stroke aesthetic. Standardize stroke width across all icons.
- Illustrations: monochromatic, rough continuous-line ink sketches on a white background, featuring a single offset geometric shape filled with a muted pastel.
- Photography: high-quality, desaturated images with a warm tone; apply subtle overlays (`opacity: 0.04` warm grain) to blend photos into the monochrome palette. Never use oversaturated stock photos. Use `https://picsum.photos/seed/{context}/1200/800` placeholders when real assets are unavailable.
- Hero and section backgrounds: sections must not feel empty and flat. Use subtle full-width imagery at very low opacity, soft radial light spots (warm `radial-gradient` at `opacity: 0.03`), or minimal geometric line patterns for depth without breaking the clean aesthetic.

## Subtle motion and micro-animations

Motion should feel invisible — present but never distracting. The goal is quiet sophistication, not spectacle.

- Scroll entry: elements fade in gently as they enter the viewport — `translateY(12px)` + `opacity: 0` resolving over `600ms` with `cubic-bezier(0.16, 1, 0.3, 1)`. Use `IntersectionObserver`, never `window.addEventListener('scroll')`.
- Hover: cards lift with an ultra-subtle shadow shift (`box-shadow` from `0 0 0` to `0 2px 8px rgba(0,0,0,0.04)` over `200ms`); buttons respond with `scale(0.98)` on `:active`.
- Staggered reveals: lists and grid items enter with a cascade delay (`animation-delay: calc(var(--index) * 80ms)`). Never mount everything at once.
- Ambient background motion (optional): a single, very slow radial gradient blob (`animation-duration: 20s+`, `opacity: 0.02-0.04`) drifting behind hero sections, applied only to a `position: fixed; pointer-events: none` layer. Never on scrolling containers.
- Performance: animate exclusively via `transform` and `opacity`; no layout-triggering properties (`top`, `left`, `width`, `height`). Use `will-change: transform` sparingly and only on actively animating elements.

## Execution order

1. Establish the macro-whitespace first: massive vertical padding between sections (`py-24` or `py-32`).
2. Constrain the main typographic content width to `max-w-4xl` or `max-w-5xl`.
3. Apply the custom typographic hierarchy and monochrome color variables immediately.
4. Ensure every card, divider, and border adheres strictly to the `1px solid #EAEAEA` rule.
5. Add scroll-entry animations to all major content blocks.
6. Give sections visual depth through imagery, ambient gradients, or subtle textures — no empty flat backgrounds.
