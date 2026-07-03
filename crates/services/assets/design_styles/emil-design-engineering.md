<!--
Adapted from the "emil-design-eng" skill in emilkowalski/skills
Source: https://github.com/emilkowalski/skills
License: MIT. See the SoloDawn LICENSE file for full attribution.
Changes: condensed and adapted for automated prompt injection (2026-07-04).
-->

# Design Engineering: Animation and Interaction Craft

Build interfaces where every detail compounds into something that feels right. Most details are never consciously noticed — that is the point: the aggregate of invisible correctness creates interfaces people love without knowing why. Beauty is leverage: good defaults and good animations are real differentiators.

## Decide whether to animate

Ask how often users will see it: 100+ times/day (keyboard shortcuts, command palette) — no animation, ever; tens of times/day (hover, list navigation) — remove or drastically reduce; occasional (modals, drawers, toasts) — standard; rare or first-time (onboarding, celebrations) — can add delight.

Never animate keyboard-initiated actions: they repeat hundreds of times daily, and animation makes them feel slow and disconnected.

Every animation must answer "why does this animate?" Valid purposes: spatial consistency (a toast enters and exits from the same direction), state indication, explanation, feedback (a button scales down on press), preventing jarring appearance/disappearance. If the purpose is only "it looks cool" and users see it often, do not animate.

## Easing

- Entering or exiting: ease-out (starts fast, feels responsive). Moving or morphing on screen: ease-in-out. Hover or color change: ease. Constant motion (marquee, progress bar): linear. Default: ease-out.
- Built-in CSS easings are too weak; use custom curves:
  - `--ease-out: cubic-bezier(0.23, 1, 0.32, 1);`
  - `--ease-in-out: cubic-bezier(0.77, 0, 0.175, 1);`
  - `--ease-drawer: cubic-bezier(0.32, 0.72, 0, 1);` (iOS-like drawer curve)
- Never use ease-in for UI animations: it delays the initial movement at the exact moment the user is watching most closely, so the interface feels sluggish.

## Duration

Button press feedback 100-160ms; tooltips and small popovers 125-200ms; dropdowns and selects 150-250ms; modals and drawers 200-500ms; marketing/explanatory can be longer. Keep UI animations under 300ms.

Perceived speed matters as much as actual speed: a 180ms select feels more responsive than a 400ms one; a fast-spinning spinner makes loading feel faster; ease-out at 200ms feels faster than ease-in at 200ms because the user sees immediate movement.

## Springs

Use springs for drag interactions with momentum, elements that should feel alive, interruptible gestures, and decorative mouse-tracking (interpolate through a spring instead of tracking position directly; if the element is functional, no animation is better). Prefer `{ type: "spring", duration: 0.5, bounce: 0.2 }` over raw mass/stiffness/damping. Keep bounce subtle (0.1-0.3); avoid it outside drag-to-dismiss and playful moments. Springs maintain velocity when interrupted — CSS keyframes restart from zero — ideal for gestures users may reverse mid-motion.

## Component rules

- Every pressable element: `transform: scale(0.97)` on `:active` with `transition: transform 160ms ease-out`; keep the scale subtle (0.95-0.98).
- Never animate from `scale(0)`: nothing real appears from nothing. Start from `scale(0.9)` or higher, combined with `opacity: 0`.
- Popovers scale in from their trigger, not center: set `transform-origin` to the trigger side (Radix: `var(--radix-popover-content-transform-origin)`). Modals are the exception — keep them centered.
- Tooltips: delay the first; open adjacent ones instantly with no delay and no animation.
- Use CSS transitions over keyframes for anything rapidly triggered (toasts, toggles): transitions retarget smoothly mid-flight; keyframes restart from zero.
- When a crossfade feels off despite easing and duration changes, add `filter: blur(2px)` during the transition: blur blends the two overlapping states into one perceived transformation. Keep blur under 20px — heavy blur is expensive, especially in Safari.
- Animate entry with `@starting-style` where supported; otherwise fall back to a `data-mounted` attribute set after first render.

## Transforms and clip-path

- Prefer percentage translates: `translateY(100%)` moves an element by its own height — less error-prone than pixels for drawers and toasts.
- `scale()` scales children too (text, icons); that is a feature, not a bug.
- `clip-path: inset()` is a first-class animation tool: reveal with `inset(0 100% 0 0)` to `inset(0 0 0 0)`; image reveals on scroll from `inset(0 0 100% 0)`; comparison sliders by driving one inset from drag position. Hold-to-delete: the press fills a clipped overlay over 2s linear; release snaps back in 200ms ease-out.

## Gestures and drag

- Momentum dismissal: velocity = |dragDistance| / elapsedTime; dismiss when velocity exceeds ~0.11 even below the distance threshold — a quick flick is enough.
- Past a natural boundary, apply damping — the more the user drags, the less it moves; prefer friction over hard stops.
- Capture pointer events once dragging starts; ignore additional touch points mid-drag.

## Performance

- Animate only `transform` and `opacity` — they skip layout and paint.
- Do not drive per-frame motion through a CSS variable on a parent (it recalculates styles for all children); set `transform` directly on the element.
- Framer Motion shorthand props (`x`, `y`, `scale`) are not hardware-accelerated; use the full `transform` string when frames must hold under load.
- CSS animations run off the main thread and stay smooth while the browser is busy: use CSS for predetermined animations, JS for dynamic ones, and the Web Animations API for programmatic control with CSS performance.

## Accessibility

- `prefers-reduced-motion` means fewer and gentler animations, not zero: keep opacity and color transitions that aid comprehension; remove movement.
- Gate hover effects behind `@media (hover: hover) and (pointer: fine)` — touch devices trigger hover on tap.

## Taste rules

- Good defaults matter more than options: easing, timing, and visual design must be excellent out of the box.
- Handle edge cases invisibly: pause timers when the tab is hidden, fill hover gaps between stacked items, protect drags from multi-touch.
- Cohesion: match motion to the component's personality — playful components can be bouncier; professional dashboards should be crisp and fast.
- Asymmetric timing: slow where the user is deciding, fast where the system responds (press 2s linear, release 200ms ease-out). Exits generally faster than entrances.
- Stagger simultaneous entrances by 30-80ms per item; stagger is decorative — never block interaction while it plays.

## Self-review checklist

`transition: all` — name exact properties. `scale(0)` entry — `scale(0.95)` + `opacity: 0`. `ease-in` on UI — ease-out or custom curve. Centered popover origin — trigger-aware (modals exempt). Animation on a keyboard action — remove. UI duration over 300ms — cut to 150-250ms. Hover without the media query — gate it. Keyframes on rapid elements — transitions. Same enter/exit speed — exit faster. Everything at once — stagger 30-80ms.
