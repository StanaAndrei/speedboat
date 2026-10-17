# Speedboat GUI / Desktop Agent Architecture & Strategy

This document outlines the architectural plan for the **Speedboat Desktop Agent**, designed specifically for **non-technical volunteers** across Windows, macOS, and Linux.

---

## 1. Executive Summary & Goals

* **Target Audience:** Casual, non-technical volunteers who want to contribute idle computer power without dealing with Docker, terminal commands, virtual machines, or complex installation steps.
* **Core Philosophy:** **Zero-friction adoption**. The volunteer downloads a single, lightweight binary (~15–25 MB), launches it, and it immediately begins crunching scientific tasks.
* **Unified Codebase:** A single, cross-platform client codebase (using **Wails + Preact**) that compiles to native desktop binaries for Windows (`.exe`), macOS (`.app` / `.dmg`), and Linux (`.AppImage` / `.deb`), with OS-specific runtime execution strategies implemented behind Go build tags.

---

## 2. Frontend & Application Shell: Wails + Preact

### Why Wails (Go + Native Webview)?
1. **Shared Go Stack:** Speedboat's agent core (`internal/agent`, `internal/protocol`, `internal/runtime`, NATS client) is written in Go. With Wails, both the UI shell and the backend execution engine share the same Go process, memory space, and types.
2. **Ultra-Lightweight Footprint:**
   * Unlike Electron (which bundles a 150+ MB Chromium runtime and Node.js), Wails leverages the host OS's pre-installed native webview:
     * **Windows:** Microsoft Edge WebView2 (built into Windows 10/11).
     * **macOS:** Native WebKit.
     * **Linux:** WebKitGTK.
   * Resulting distribution package is only **~15–25 MB**.
3. **System Tray & 24/7 Background Computing:**
   * Ability to minimize to the system tray on close.
   * Auto-start on system boot.
   * Low baseline idle memory footprint (< 50 MB RAM).

### UI Capabilities (Preact Dashboard)
* **Real-time Metrics:** Active task name, computation progress percentage, CPU utilization, and completed task counters.
* **Volunteer Controls & Throttles:**
  * CPU slider: Select max allowed CPU threads (e.g., 2, 4, 8 threads).
  * Power-aware execution: Auto-pause when running on battery (laptops).
  * Activity-aware execution: Auto-pause when user is actively gaming or running high-load foreground applications.
* **Gamification & Retention:** Team badges, rank, leaderboard, and accumulated volunteer compute points.

---

## 3. Execution Tier Architecture: Multi-Tier Volunteer Runtime

Because low-level OCI container runtimes (`runc` and `crun`) depend directly on Linux kernel primitives (namespaces, cgroups, pivot_root) and cannot run natively on Windows NT or macOS Darwin, Speedboat adopts a tiered execution model:

```
+-----------------------------------------------------------------------------------+
|                           Speedboat Desktop UI (Wails)                           |
+-----------------------------------------------------------------------------------+
                                         |
                                         v
+-----------------------------------------------------------------------------------+
|                        Unified Agent Orchestrator (Go)                            |
+-----------------------------------------------------------------------------------+
       |                                          |                              |
       v (Default / Zero-Install)                 v (Linux Native)               v (Opt-in / Heavy)
+-------------------------------+      +----------------------+      +----------------------+
|       Tier 1: Lite Mode       |      |    Tier 2: Linux     |      |  Tier 3: Heavy Mode  |
|   Embedded wazero (Pure Go)   |      |   Rootless crun /    |      | WSL2 (Win) / VM (Mac)|
|-------------------------------|      |      apptainer       |      | Docker / Podman      |
| • 100% Zero Install           |      |----------------------|      |----------------------|
| • Runs WASI-compiled binaries |      | • User namespaces    |      | • Full OCI images    |
| • Near-native JIT speed       |      | • No Docker daemon   |      | • GPU acceleration   |
| • In-memory sandbox security  |      | • Direct host exec   |      | • Setup/Admin wizard |
+-------------------------------+      +----------------------+      +----------------------+
```

---

## 4. Tier 1: Embedded `wazero` (The Universal Baseline)

### Role
The primary compute engine for all non-technical users on **Windows and macOS**, and a fallback on Linux.

### Key Characteristics
* **Pure Go (Zero CGO, Zero DLLs, Zero External Runtimes):**
  * `github.com/tetratelabs/wazero` statically compiles directly into `agent.exe` and `Speedboat.app`.
* **Direct Compatibility with WASI SDK:**
  * Scientific code written in C, C++, Rust, or Python is compiled to WebAssembly via the **WASI SDK** (`clang -target wasm32-wasi`).
  * `wazero` implements the full WASI snapshot specification (`wasi_snapshot_preview1`), seamlessly satisfying Speedboat's standard stream contract (`stdin` -> compute -> `stdout`).
* **Silent In-Memory JIT:**
  * Compiles `.wasm` bytecode in-memory to native machine code (x86-64 / ARM64), achieving ~85–90% of native compute speed.
* **Strict Sandboxing:**
  * Memory is isolated to the WASM linear memory jail.
  * Zero access to host filesystem, registry, or local network. Crashes or memory traps in scientific algorithms are caught as Go errors without crashing the agent.

---

## 5. Tier 2 & 3: Container Strategy by Platform

### A. Linux (Straightforward Rootless Containers)
* Non-technical Linux volunteers do not need Docker.
* The agent uses rootless user namespaces via **`crun`**, **`bubblewrap` (`bwrap`)**, or **`apptainer`**.
* Direct access to host filesystems (with sandbox flags) and host GPU passthrough where available.

### B. Windows: Automated WSL2 Setup & Fallback Plan
On Windows, true Linux container execution requires a Linux kernel. While technical users run Docker or manual WSL2, non-technical volunteers require automation:

1. **Detection on Startup:**
   * The app checks whether a container runtime already exists (`docker`, `podman`, or active `wsl`).
   * If detected: The agent exposes an option to enable **High Performance / Heavy Mode** (allowing container tasks and CUDA GPU compute).
2. **If Not Detected: Stay in Tier 1 (wazero):**
   * The app immediately starts computing WASI/WASM tasks with **zero setup prompts**.
3. **One-Click WSL2 Automated Provisioning (Optional / Opt-in):**
   * For projects requiring full Linux containers, the UI presents a friendly one-click wizard:
     > *"Enable Full Container & GPU Support"*
   * Behind the scenes, the Go desktop agent automates:
     1. Invoking `wsl --install --no-distribution` (handles feature flags).
     2. Downloading a minimal **Alpine Linux rootfs with `crun`** (~5–10 MB, avoiding full 1 GB Ubuntu images).
     3. Importing the distribution silently:
        ```cmd
        wsl --import SpeedboatWorker "%LOCALAPPDATA%\Speedboat\wsl" alpine-crun.tar.gz --version 2
        ```
     4. Configuring `.wslconfig` to cap memory usage so it never starves the volunteer's host system.
     5. Running tasks headless inside WSL:
        ```cmd
        wsl -d SpeedboatWorker -- crun run ...
        ```
   * **Handling Windows Friction:**
     * **Admin Rights (UAC):** The wizard informs the user that Windows requires administrative approval.
     * **Reboots:** If Hypervisor/Virtual Machine Platform was not enabled, the UI guides the user through the required one-time reboot.
     * **BIOS Virtualization Disabled:** If VT-x/AMD-V is disabled in motherboard firmware, the app gracefully falls back to **Tier 1 (wazero)** so the volunteer can still contribute compute without troubleshooting BIOS settings.

### C. macOS (Darwin Strategy)
* **Tier 1 (Default):** Runs embedded `wazero` (optimized for Apple Silicon ARM64 with near-native speed).
* **Tier 2 (Heavy Mode):**
  * Detect existing Docker Desktop or Colima installations.
  * Future roadmap: Embed a lightweight microVM using macOS's native `Virtualization.framework` (e.g., via `libkrun` / `vfkit`) running a headless Linux kernel without requiring third-party hypervisors.

---

## 6. Wazero on Desktop vs. Browser WASM: Why Local Desktop Wins

While Speedboat can offer an optional "instant try" web page using in-browser WASM, the **Wails desktop agent with wazero is the primary compute workhorse**:

| Metric | Desktop Wazero (`agent.exe` / Wails) | Browser WASM (`nats.ws` in Tab) |
| :--- | :--- | :--- |
| **Tab / Window Throttling** | None; runs at full OS thread priority | Severe; browsers throttle inactive/hidden tabs down to 1–5% CPU |
| **Volunteer Persistence** | 24/7 background execution via system tray & startup | Ephemeral; terminates when tab or browser is closed |
| **Memory Capacity** | Direct host memory (unrestricted) | Browsers enforce strict per-tab memory caps (often 2–4 GB) |
| **Network Protocol** | Native TCP / TLS via `nats.go` (efficient) | Forced WebSocket framing (`nats.ws`), requiring extra proxies |
| **System Awareness** | Detects battery vs. AC power, gaming, CPU usage | Sandboxed from hardware and battery state APIs |
| **Local Cache** | Direct multi-gigabyte disk cache for models and inputs | Subject to browser storage quotas and cache evictions |

---

## 7. Implementation Roadmap & Architecture Milestones

1. **Phase 1: Wazero Engine Integration**
   * Implement `internal/runtime/wazero.go` satisfying the `runtime.Engine` interface.
   * Wire standard stream contracts (`stdin` JSON -> WASI execution -> `stdout` JSON).
   * Verify end-to-end task execution using WASI SDK compiled artifacts.
2. **Phase 2: Headless Cross-Platform Agent Verification**
   * Ensure `cmd/agent` compiles and runs cleanly across Windows, macOS, and Linux with the wazero engine.
3. **Phase 3: Wails Desktop Shell (Preact Frontend)**
   * Initialize Wails project structure wrapping the existing Go agent.
   * Build the Preact dashboard: live status, task throughput, CPU throttling slider, and system tray integration.
4. **Phase 4: Heavy Mode Auto-Detection & Windows WSL2 Wizard**
   * Add runtime environment probing (check for Docker, Podman, WSL2).
   * Implement the optional one-click Alpine WSL2 provisioning wizard for advanced Windows tasks.


# On Linux / WSL to win:
    sudo apt install gcc-mingw-w64
    wails build -platform windows/amd64

