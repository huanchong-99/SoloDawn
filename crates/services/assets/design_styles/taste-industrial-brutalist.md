<!--
Adapted from the "industrial-brutalist-ui" skill in Leonxlnx/taste-skill
Source: https://github.com/Leonxlnx/taste-skill
License: MIT. See the SoloDawn LICENSE file for full attribution.
Changes: condensed and adapted for automated prompt injection (2026-07-04).
-->

# Industrial Brutalism and Tactical Telemetry UI

Architect web interfaces that synthesize mid-century Swiss typographic design, industrial manufacturing manuals, and retro-futuristic aerospace/military terminal interfaces. Enforce rigid modular grids, extreme typographic scale contrast, purely utilitarian color, and programmatic simulation of analog degradation (halftones, CRT scanlines, bitmap dithering). The result must project raw functionality, mechanical precision, and high data density, deliberately discarding conventional consumer UI patterns.

## Visual archetypes — pick ONE per project and commit

Never alternate or mix both modes within the same interface.

- Swiss Industrial Print: high-contrast light mode on newsprint/off-white substrates; monolithic, heavy sans-serif typography; unforgiving structural grids outlined by visible dividing lines; aggressive, asymmetric negative space punctuated by oversized, viewport-bleeding numerals or letterforms; heavy use of primary red as the alert/accent color.
- Tactical Telemetry / CRT Terminal: dark mode exclusively; high-density tabular data presentation; absolute dominance of monospaced typography; technical framing devices (ASCII brackets, crosshairs); simulated hardware limitations (phosphor glow, scanlines, low bit-depth rendering).

## Typographic architecture

Typography is the primary structural and decorative infrastructure; imagery is secondary. Demand extreme variance in scale, weight, and spacing.

- Macro-typography (structural headers): neo-grotesque / heavy sans-serif — Neue Haas Grotesk (Black), Inter (Extra Bold/Black), Archivo Black, Roboto Flex (Heavy), Monument Extended. Deploy at massive fluid scales (`clamp(4rem, 10vw, 15rem)`); tracking extremely tight, often negative (`-0.03em` to `-0.06em`) so glyphs form solid architectural blocks; line-height highly compressed (`0.85` to `0.95`); exclusively uppercase.
- Micro-typography (data and telemetry): monospace / technical sans — JetBrains Mono, IBM Plex Mono, Space Mono, VT323, Courier Prime. Fixed small scale (`10px` to `14px`); generous tracking (`0.05em` to `0.1em`) to simulate typewriter or terminal matrices; leading `1.2` to `1.4`; exclusively uppercase. Use for all metadata, navigation, unit IDs, and coordinates.
- Textural contrast (use exceedingly sparingly): high-contrast serif — Playfair Display, EB Garamond, Times New Roman — always subjected to heavy post-processing (halftone filters, 1-bit dithering) to degrade vector perfection against the clean sans-serifs.

## Color system — choose ONE substrate palette, never mix

Gradients, soft drop shadows, and modern translucency are strictly prohibited. Colors simulate physical media or primitive emissive displays.

- Swiss Industrial Print (light): background `#F4F4F0` or `#EAE8E3` (matte, unbleached documentation paper); foreground `#050505` to `#111111` (carbon ink); accent `#E61919` or `#FF2A2A` (aviation/hazard red) — the ONLY accent color, used for strike-throughs, thick structural dividing lines, or vital data highlights.
- Tactical Telemetry (dark): background `#0A0A0A` or `#121212` (deactivated CRT — avoid pure `#000000`); foreground `#EAEAEA` (white phosphor) as the primary text color; the same red accent under the same rules. Terminal green `#4AF626` is optional: use it ONLY for a single specific UI element (one status indicator or one data readout), never as a general text color; if it serves no clear purpose, omit it entirely.

## Layout and spatial engineering

The layout must appear mathematically engineered; reject conventional web padding in favor of visible compartmentalization.

- Blueprint grid: strict CSS Grid architecture. Elements do not float; anchor them precisely to grid tracks and intersections.
- Visible compartmentalization: extensive solid borders (`1px` or `2px solid`) delineating distinct zones of information; horizontal rules frequently spanning the entire container width to segregate operational units.
- Bimodal density: oscillate between extreme data density (tightly packed monospace metadata) and vast expanses of calculated negative space framing macro-typography.
- Geometry: absolute rejection of `border-radius`. All corners exactly 90 degrees to enforce mechanical rigidity.

## Components and symbology

Replace standard web UI conventions with utilitarian, industrial graphic elements.

- Syntax decoration: frame data points with ASCII — `[ DELIVERY SYSTEMS ]`, `< RE-IND >`; directional marks `>>>`, `///`, `\\\\`.
- Industrial markers: registration `(R)`, copyright `(C)`, and trademark `(TM)` symbols used prominently as structural geometric elements, not legal text.
- Technical assets: crosshairs (`+`) at grid intersections, repeating vertical barcode lines, thick horizontal warning stripes, and randomized string data (`REV 2.6`, `UNIT / D-01`) to simulate active mechanical processes.

## Textural and post-processing effects

Engineer simulated analog degradation into the frontend via CSS and SVG filters so the design never appears purely digital.

- Halftone / 1-bit dithering: transform continuous-tone images or large serif typography into dot-matrix patterns via pre-processing or CSS `mix-blend-mode: multiply` overlays combined with SVG radial dot patterns.
- CRT scanlines (terminal mode): apply `repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(0,0,0,0.1) 2px, rgba(0,0,0,0.1) 4px)` to the background to simulate horizontal electron beam sweeps.
- Mechanical noise: one global, low-opacity SVG static/noise filter on the DOM root for unified physical grain across both dark and light modes.

## Web engineering directives

1. Grid determinism: use `display: grid; gap: 1px;` with contrasting parent/child background colors to generate mathematically perfect, razor-thin dividing lines without complex border declarations.
2. Semantic rigidity: construct the DOM with precise semantic tags (`<data>`, `<samp>`, `<kbd>`, `<output>`, `<dl>`) to accurately reflect the technical nature of the telemetry.
3. Typography clamping: implement CSS `clamp()` exclusively for macro-typography so massive text scales aggressively while maintaining structural integrity across viewports.
