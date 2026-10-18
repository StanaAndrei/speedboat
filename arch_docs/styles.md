# Speedboat Design System: Hydrofoil HyperGrid

The **Hydrofoil HyperGrid** design system defines the aesthetic, sensory, and functional identity for the Speedboat distributed volunteer computing platform across both the **Browser Web Worker** client (`internal/orchestrator/web`) and the **Wails Desktop Agent** (`gui/frontend`).

---

## 1. Design Philosophy & Thematic Identity

Speedboat decouples scientific orchestrators from untrusted volunteer worker nodes. Unlike traditional volunteer platforms designed as utilitarian 2000s desktop utilities, Speedboat's identity reflects **extreme speed, fluid dynamics, and aerospace-grade telemetry**:

- **The Supersonic Hydrofoil:** The visual metaphor of a high-speed hydrofoil vessel cutting through turbulent waters into calm, ultra-fast laminar flow.
- **Cockpit Telemetry:** Every panel, card, and indicator is designed like high-precision avionics and telemetry instrumentation—clean, readable at a glance, and alive with real-time computational flow.
- **Gamified Volunteerism:** Volunteers are not just running background daemons; they are *piloting nodes* in a planetary hypergrid. Live compute credits, pilot rank progression, and hardware telemetry dials create retention and visual reward.
- **Zero Friction, Modern Glassmorphism:** Deep obsidian stealth hull (`#060913`) layered with translucent cyber-mesh panels, crisp electric cyan (`#00f0ff`) glow trails, and plasma orchid (`#a855f7`) accents.

---

## 2. Design Tokens & Color Palette

### Core Color Palette

| Token | Hex / Value | Role / Usage |
| :--- | :--- | :--- |
| `--bg-hull` | `#060913` | Deep obsidian canvas background |
| `--bg-surface` | `#0c1220` | Secondary surface background (controls, inputs) |
| `--card-bg` | `rgba(14, 21, 37, 0.75)` | Translucent glassmorphic card backdrop |
| `--card-border` | `rgba(56, 189, 248, 0.16)` | Default panel border with subtle cyan tint |
| `--card-border-hover`| `rgba(0, 240, 255, 0.45)` | Focused/hovered card border glow |
| `--wake-cyan` | `#00f0ff` | Primary action accent, active wake glow |
| `--wake-cyan-glow` | `rgba(0, 240, 255, 0.28)` | Radiant aura around primary buttons and badges |
| `--plasma-violet` | `#a855f7` | Secondary accent, rank tiers, heavy compute |
| `--plasma-glow` | `rgba(168, 85, 247, 0.25)`| Radiant aura for heavy mode & high-tier badges |
| `--quantum-emerald` | `#10b981` | Success state, consensus verified, node idle/ready |
| `--emerald-glow` | `rgba(16, 185, 129, 0.3)` | Pulse glow for verified status |
| `--solar-amber` | `#f59e0b` | Connecting state, warnings, throttle indicators |
| `--warp-coral` | `#f43f5e` | Danger, errors, disconnect actions |
| `--text-primary` | `#f8fafc` | Titanium white high-contrast headers & values |
| `--text-secondary` | `#94a3b8` | Tactical slate for secondary descriptions |
| `--text-muted` | `#64748b` | Muted telemetry timestamps, captions, labels |
| `--terminal-bg` | `rgba(7, 10, 18, 0.88)` | High-contrast console background |

### Atmospheric Background
```css
body {
  background-color: var(--bg-hull);
  background-image: 
    radial-gradient(ellipse 80% 50% at 50% -20%, rgba(0, 240, 255, 0.12), transparent),
    radial-gradient(ellipse 60% 40% at 100% 100%, rgba(168, 85, 247, 0.08), transparent),
    linear-gradient(rgba(255, 255, 255, 0.02) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.02) 1px, transparent 1px);
  background-size: 100% 100%, 100% 100%, 32px 32px, 32px 32px;
}
```

---

## 3. Typography & Numerical Precision

- **Font Family:**
  - Primary UI: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, system-ui, sans-serif`
  - Telemetry & Terminal: `ui-monospace, "SF Mono", "JetBrains Mono", Menlo, Consolas, monospace`
- **Numerical Formatting:** All telemetry counters, task totals, and latency metrics enforce `font-variant-numeric: tabular-nums` to eliminate jitter during rapid live updates.
- **Micro-Labels:** `.stat-title`, `.telemetry-tag`, and section labels use `text-transform: uppercase`, `letter-spacing: 0.08em`, and bold weights (`600–700`) at `0.72rem–0.78rem` for a cockpit readout feel.

---

## 4. Visual Components

### A. Supersonic Hydrofoil Emblem
Both applications embed a precision SVG emblem featuring:
- An aerodynamic wedge hull slicing vertically forward.
- Twin hydrodynamic wake lines expanding laterally.
- Glowing cyan/violet dual thruster trail.

```html
<svg class="brand-logo" viewBox="0 0 36 36" fill="none">
  <!-- Glowing thruster trails -->
  <path d="M14 28L18 34L22 28" stroke="url(#thruster-glow)" stroke-width="2.5" stroke-linecap="round"/>
  <!-- Aerodynamic hydrofoil hull -->
  <polygon points="18,3 8,24 18,20 28,24" fill="url(#hull-gradient)" stroke="#00f0ff" stroke-width="1.2"/>
  <!-- Cockpit canopy -->
  <polygon points="18,9 14,19 18,17 22,19" fill="#00f0ff" fill-opacity="0.85"/>
</svg>
```

### B. Telemetry Cards (`.stat-card`)
- Built with multi-layer glassmorphism:
  - `backdrop-filter: blur(14px)`
  - Border: `1px solid var(--card-border)`
  - Inset highlight: `box-shadow: inset 0 1px 1px rgba(255, 255, 255, 0.06)`
- Hover transition: Translates `translateY(-2px)` with expanded cyan shadow `0 8px 24px rgba(0, 240, 255, 0.12)`.

### C. Live Telemetry Pulse & Status Badges
Status indicators use an animated multi-ring radar pulse:
- **Computing:** Electric cyan pulse with expanding ripple waves (`animation: ripple 1.8s infinite cubic-bezier(0, 0.2, 0.8, 1)`).
- **Idle / Ready:** Quantum emerald solid dot with soft aura.
- **Connecting:** Solar amber pulsing dot.
- **Paused:** Muted slate dot.

### D. Gamification Framework
As outlined in `arch_docs/project.txt`, volunteer networks thrive on retention metrics:
1. **Compute Credits (FLOPS Ticker):**
   $$\text{Credits} = (\text{Tasks Completed} \times 100) + \left\lfloor \frac{\text{Total Compute Milliseconds}}{100} \right\rfloor$$
2. **Pilot Rank Tiers:**
   - `0–4 Tasks`: **Cadet Pilot** (Slate Badge)
   - `5–19 Tasks`: **Hydrofoil Specialist** (Cyan Badge)
   - `20–49 Tasks`: **Supersonic Voyager** (Violet Badge)
   - `50–199 Tasks`: **Quantum Fleet Commander** (Emerald Glow Badge)
   - `200+ Tasks`: **Mach-10 Grand Architect** (Gold Aura Badge)

### E. Cyber Telemetry Console (Terminal HUD)
- Header featuring:
  - Mock terminal dots with glowing cyan activity LED.
  - Interactive log level filter tabs (`ALL`, `INFO`, `WARNS`, `ERRORS`).
  - Auto-scroll lock toggle switch.
  - One-click "Copy Logs" button with animated checkmark confirmation.
  - "Clear" terminal action.
- Body with subtle CRT scanlines overlay, timestamp syntax coloring, and smooth custom scrollbars.

### F. Mode Selector (Wails App)
Interactive dual-choice selector between:
- **⚡ Lite Mode (Wazero WASM):** Zero-install, in-memory sandboxed execution.
- **🚀 Heavy Mode (Apptainer OCI):** Direct Linux user-namespace container execution with optional GPU passthrough.
Selected cards illuminate with active neon border rings and backdrop glow.

---

## 5. Micro-Interactions & Motion Design

1. **Button Ripple & Glow:** Buttons utilize smooth CSS `transform` and `box-shadow` transitions (`0.2s cubic-bezier(0.16, 1, 0.3, 1)`).
2. **Tab Switch & Filter Animation:** Filter pills feature active sliding border indicators and subtle scale changes.
3. **Accessibility (`prefers-reduced-motion`):** All pulse animations, radar sweeps, and card lifts gracefully fall back to static opacities when reduced motion is preferred by the operating system.

---

## 6. Implementation Map

| File | Component / Role |
| :--- | :--- |
| [`arch_docs/styles.md`](file:///home/stade/repos/speedboat/arch_docs/styles.md) | Central style system specification |
| [`internal/orchestrator/web/styles.css`](file:///home/stade/repos/speedboat/internal/orchestrator/web/styles.css) | Web Worker client styles & responsive tokens |
| [`internal/orchestrator/web/index.html`](file:///home/stade/repos/speedboat/internal/orchestrator/web/index.html) | Web Worker markup, brand SVG, and telemetry layout |
| [`gui/frontend/src/style.css`](file:///home/stade/repos/speedboat/gui/frontend/src/style.css) | Wails desktop app styling, modals, and mode selectors |
| [`gui/frontend/src/App.jsx`](file:///home/stade/repos/speedboat/gui/frontend/src/App.jsx) | Preact desktop component logic, gamification & telemetry HUD |
