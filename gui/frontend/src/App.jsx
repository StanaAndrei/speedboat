import { useState, useEffect, useRef } from 'preact/hooks';

export function App() {
  const [systemInfo, setSystemInfo] = useState({ num_cpu: 4, default_threads: 2, worker_id: 'loading...' });
  const [engineCaps, setEngineCaps] = useState(null);
  const [computeMode, setComputeMode] = useState('wasm'); // 'wasm' | 'apptainer'
  const [enableGPU, setEnableGPU] = useState(false);

  const [projectUrl, setProjectUrl] = useState('');
  const [preview, setPreview] = useState(null);
  const [isResolving, setIsResolving] = useState(false);
  const [concurrency, setConcurrency] = useState(2);
  const [isConnected, setIsConnected] = useState(false);
  const [isPaused, setIsPaused] = useState(false);
  const [status, setStatus] = useState({ state: 'idle', text: 'Ready to connect' });
  const [metrics, setMetrics] = useState({ tasks_completed: 0, total_compute_ms: 0, active_threads: 0, average_latency_ms: 0 });
  const [logs, setLogs] = useState([]);
  const [autoScroll, setAutoScroll] = useState(true);

  // Modal states
  const [showInstallModal, setShowInstallModal] = useState(false);
  const [showPlatformModal, setShowPlatformModal] = useState(false);
  const [isInstalling, setIsInstalling] = useState(false);
  const [installProgress, setInstallProgress] = useState({ percent: 0, message: '' });

  const terminalRef = useRef(null);

  // Refresh engine capabilities
  const refreshCaps = async () => {
    if (window.go && window.go.main && window.go.main.App) {
      try {
        const caps = await window.go.main.App.GetEngineCapabilities();
        setEngineCaps(caps);
        if (caps.gpu_available) {
          setEnableGPU(true);
        }
      } catch (e) {
        console.error('Failed to get engine caps:', e);
      }
    }
  };

  // Initialize System Info and Wails event listeners
  useEffect(() => {
    if (window.go && window.go.main && window.go.main.App) {
      window.go.main.App.GetSystemInfo().then((info) => {
        setSystemInfo(info);
        if (info.engine_caps) {
          setEngineCaps(info.engine_caps);
          if (info.engine_caps.gpu_available) {
            setEnableGPU(true);
          }
        }
        setConcurrency(info.default_threads || 2);
      });
    }

    if (window.runtime && window.runtime.EventsOn) {
      window.runtime.EventsOn('log', (logEntry) => {
        setLogs((prev) => [...prev.slice(-300), logEntry]);
      });

      window.runtime.EventsOn('status', (statusEntry) => {
        setStatus(statusEntry);
        if (statusEntry.state === 'paused') {
          setIsPaused(true);
        } else if (statusEntry.state === 'computing' || statusEntry.state === 'idle') {
          setIsPaused(false);
        }
      });

      window.runtime.EventsOn('metrics', (metricsData) => {
        setMetrics(metricsData);
      });

      window.runtime.EventsOn('install_progress', (progressData) => {
        setInstallProgress(progressData);
      });
    }

    // Auto-check clipboard for Speedboat link on startup
    checkClipboard();
  }, []);

  // Auto-scroll terminal
  useEffect(() => {
    if (autoScroll && terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  const checkClipboard = async () => {
    try {
      if (navigator.clipboard && navigator.clipboard.readText) {
        const text = await navigator.clipboard.readText();
        if (text && (text.includes('project=') || text.includes(':3000'))) {
          setProjectUrl(text.trim());
          handleResolve(text.trim());
        }
      }
    } catch (_) {
      // Clipboard read may not be permitted without user gesture
    }
  };

  const handleResolve = async (urlToResolve) => {
    const target = urlToResolve || projectUrl;
    if (!target.trim()) return;

    setIsResolving(true);
    try {
      if (window.go && window.go.main && window.go.main.App) {
        const res = await window.go.main.App.ResolveProjectURL(target.trim());
        setPreview(res);
      } else {
        // Fallback for standalone browser testing
        setPreview({
          project_id: 'prime-counter',
          nats_ws_url: 'ws://127.0.0.1:8080',
          payload_url: 'http://127.0.0.1:3000/payload.wasm',
          master_host: '127.0.0.1:3000',
          reachable: true,
          status_text: 'Online (Mock)',
        });
      }
    } catch (err) {
      setPreview({
        project_id: 'unknown',
        nats_ws_url: '',
        master_host: '',
        reachable: false,
        status_text: String(err),
      });
    } finally {
      setIsResolving(false);
    }
  };

  const handleSelectHeavyMode = () => {
    if (!engineCaps) {
      setComputeMode('apptainer');
      return;
    }

    if (!engineCaps.heavy_mode_supported) {
      setShowPlatformModal(true);
      return;
    }

    if (!engineCaps.is_installed) {
      setShowInstallModal(true);
      return;
    }

    setComputeMode('apptainer');
  };

  const handleRunInstall = async () => {
    if (!window.go || !window.go.main || !window.go.main.App) return;

    setIsInstalling(true);
    setInstallProgress({ percent: 5, message: 'Starting installation...' });

    try {
      await window.go.main.App.InstallHeavyEngine();
      await refreshCaps();
      setComputeMode('apptainer');
      setShowInstallModal(false);
    } catch (err) {
      setInstallProgress({ percent: 100, message: `Installation failed: ${err}` });
    } finally {
      setIsInstalling(false);
    }
  };

  const handleStart = async () => {
    if (!preview) return;

    setIsConnected(true);
    setLogs([]);
    setStatus({ state: 'connecting', text: 'Initializing...' });

    if (window.go && window.go.main && window.go.main.App) {
      try {
        await window.go.main.App.StartCompute({
          project_id: preview.project_id,
          nats_url: preview.nats_ws_url,
          payload_url: preview.payload_url,
          concurrency: parseInt(concurrency, 10),
          compute_mode: computeMode,
          enable_gpu: enableGPU,
        });
      } catch (err) {
        setStatus({ state: 'paused', text: 'Connection Error' });
      }
    }
  };

  const handleTogglePause = async () => {
    if (!window.go || !window.go.main) return;
    if (isPaused) {
      await window.go.main.App.ResumeCompute();
      setIsPaused(false);
    } else {
      await window.go.main.App.PauseCompute();
      setIsPaused(true);
    }
  };

  const handleConcurrencyChange = async (e) => {
    const val = parseInt(e.target.value, 10);
    setConcurrency(val);
    if (isConnected && window.go && window.go.main) {
      await window.go.main.App.SetConcurrency(val);
    }
  };

  const handleStop = async () => {
    if (window.go && window.go.main) {
      await window.go.main.App.StopCompute();
    }
    setIsConnected(false);
    setIsPaused(false);
    setStatus({ state: 'idle', text: 'Disconnected' });
  };

  const formatDuration = (ms) => {
    if (ms < 1000) return `${ms} ms`;
    return `${(ms / 1000).toFixed(1)} s`;
  };

  return (
    <div class="container">
      <header>
        <div class="brand">
          <svg viewBox="0 0 24 24">
            <path d="M12 2L2 22h20L12 2zm0 3.8L18.5 19H5.5L12 5.8zM11 11v4h2v-4h-2zm0 6v2h2v-2h-2z" />
          </svg>
          <div>
            <h1>Speedboat Desktop</h1>
            <p style="font-size: 0.85rem; color: var(--muted);">High-Speed Distributed Volunteer Grid</p>
          </div>
        </div>

        <div class="status-badge">
          <span class={`dot ${status.state}`}></span>
          <span>{status.text}</span>
        </div>
      </header>

      {!isConnected ? (
        /* CONNECT VIEW */
        <div class="connect-card">
          <div class="connect-header">
            <h2>Connect to Speedboat Project</h2>
            <p>
              Paste the invite link emitted by the master node or shared by the project scientist.
            </p>
          </div>

          <div class="input-group">
            <input
              type="text"
              class="input-field"
              placeholder="e.g. http://192.168.1.138:3000/?project=prime-counter&nats=ws://192.168.1.138:8080"
              value={projectUrl}
              onInput={(e) => {
                setProjectUrl(e.target.value);
                handleResolve(e.target.value);
              }}
            />
            <button class="btn btn-outline" onClick={checkClipboard} title="Paste from clipboard">
              Paste Link
            </button>
          </div>

          {preview && (
            <div class="preview-card">
              <div class="preview-row">
                <span class="preview-label">Project:</span>
                <span class="preview-value" style="color: var(--accent); font-size: 1.1rem;">
                  {preview.project_id}
                </span>
              </div>
              <div class="preview-row">
                <span class="preview-label">Master Host:</span>
                <span class="preview-value">{preview.master_host || '-'}</span>
              </div>
              <div class="preview-row">
                <span class="preview-label">NATS Protocol:</span>
                <span class="preview-value">{preview.nats_ws_url || '-'}</span>
              </div>
              <div class="preview-row">
                <span class="preview-label">Node Status:</span>
                <span class={preview.reachable ? 'badge-online' : 'badge-offline'}>
                  <span class={`dot ${preview.reachable ? 'idle' : 'paused'}`}></span>
                  {preview.status_text}
                </span>
              </div>
            </div>
          )}

          {/* Compute Mode Selector */}
          <div class="mode-selector">
            <button
              type="button"
              class={`mode-btn ${computeMode === 'wasm' ? 'active' : ''}`}
              onClick={() => setComputeMode('wasm')}
            >
              <span class="mode-icon">⚡</span>
              <div class="mode-info">
                <div class="mode-title">Lite Mode (Wazero WASM)</div>
                <div class="mode-desc">Zero-install, instant in-memory sandboxed execution</div>
              </div>
            </button>

            <button
              type="button"
              class={`mode-btn ${computeMode === 'apptainer' ? 'active' : ''}`}
              onClick={handleSelectHeavyMode}
            >
              <span class="mode-icon">🚀</span>
              <div class="mode-info">
                <div class="mode-title">Heavy Mode ({engineCaps?.engine_name || 'Apptainer'})</div>
                <div class="mode-desc">
                  Native container throughput
                  {engineCaps?.gpu_available ? ` • ${engineCaps.gpu_vendor} GPU` : ''}
                </div>
              </div>
            </button>
          </div>

          {/* Engine status details for Heavy Mode */}
          {computeMode === 'apptainer' && engineCaps && (
            <div class="engine-status-row">
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: var(--success);">✓</span>
                <span style="color: #94a3b8;">{engineCaps.status_message}</span>
              </div>
              {engineCaps.gpu_available && (
                <label class="gpu-toggle">
                  <input
                    type="checkbox"
                    checked={enableGPU}
                    onChange={(e) => setEnableGPU(e.target.checked)}
                  />
                  <span>Enable GPU Passthrough ({engineCaps.gpu_vendor})</span>
                </label>
              )}
            </div>
          )}

          <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 1rem;">
            <div class="slider-container">
              <span class="slider-label">CPU Cores:</span>
              <input
                type="range"
                class="slider-input"
                min="1"
                max={systemInfo.num_cpu || 8}
                value={concurrency}
                onInput={handleConcurrencyChange}
              />
              <span class="slider-badge">{concurrency} Core{concurrency > 1 ? 's' : ''}</span>
            </div>

            <button
              class="btn btn-primary"
              disabled={!preview || !preview.reachable || isResolving}
              onClick={handleStart}
            >
              Start Computing
            </button>
          </div>
        </div>
      ) : (
        /* DASHBOARD VIEW */
        <>
          <div class="stats-grid">
            <div class="stat-card">
              <div class="stat-title">Project</div>
              <div class="stat-value" style="font-size: 1.3rem; color: var(--accent);">
                {preview ? preview.project_id : '-'}
              </div>
            </div>
            <div class="stat-card">
              <div class="stat-title">Runtime Engine</div>
              <div class="stat-value" style="font-size: 1.1rem; color: #38bdf8;">
                {computeMode === 'apptainer'
                  ? `🚀 Apptainer ${enableGPU ? '(+GPU)' : ''}`
                  : '⚡ Wazero WASM'}
              </div>
            </div>
            <div class="stat-card">
              <div class="stat-title">Tasks Completed</div>
              <div class="stat-value">{metrics.tasks_completed}</div>
            </div>
            <div class="stat-card">
              <div class="stat-title">Compute Time</div>
              <div class="stat-value">{formatDuration(metrics.total_compute_ms)}</div>
            </div>
          </div>

          <div class="controls-bar">
            <div class="action-buttons">
              <button
                class={`btn ${isPaused ? 'btn-primary' : 'btn-outline'}`}
                onClick={handleTogglePause}
              >
                {isPaused ? '▶ Resume Compute' : '⏸ Pause Compute'}
              </button>
              <button class="btn btn-danger" onClick={handleStop}>
                ⏹ Disconnect
              </button>
            </div>

            <div class="slider-container">
              <span class="slider-label">Compute Threads:</span>
              <input
                type="range"
                class="slider-input"
                min="1"
                max={systemInfo.num_cpu || 8}
                value={concurrency}
                onInput={handleConcurrencyChange}
              />
              <span class="slider-badge">{concurrency} Thread{concurrency > 1 ? 's' : ''}</span>
            </div>
          </div>

          <div class="terminal-container">
            <div class="terminal-header">
              <div class="terminal-dots">
                <span></span><span></span><span></span>
              </div>
              <div class="terminal-title">
                {computeMode === 'apptainer' ? 'apptainer-container-log' : 'wazero-wasm-log'} ({systemInfo.worker_id})
              </div>
              <div style="display: flex; gap: 0.75rem; align-items: center;">
                <label style="font-size: 0.75rem; color: var(--muted); display: flex; align-items: center; gap: 0.3rem; cursor: pointer;">
                  <input
                    type="checkbox"
                    checked={autoScroll}
                    onChange={(e) => setAutoScroll(e.target.checked)}
                  /> Auto-scroll
                </label>
                <button
                  class="btn btn-outline"
                  style="padding: 0.2rem 0.5rem; font-size: 0.75rem;"
                  onClick={() => setLogs([])}
                >
                  Clear
                </button>
              </div>
            </div>
            <div class="terminal-body" ref={terminalRef}>
              {logs.length === 0 ? (
                <div style="color: var(--muted); font-style: italic;">Awaiting compute tasks...</div>
              ) : (
                logs.map((l, idx) => (
                  <div key={idx} class="log-line">
                    <span class="log-time">[{l.time}]</span>
                    <span class={`log-${l.level}`}>{l.msg}</span>
                  </div>
                ))
              )}
            </div>
          </div>
        </>
      )}

      {/* 1-Click Install Modal */}
      {showInstallModal && (
        <div class="modal-backdrop">
          <div class="modal-card">
            <div class="modal-header">
              <h3>🚀 Install Apptainer Container Engine</h3>
            </div>
            <div class="modal-body">
              <p style="margin-bottom: 0.75rem;">
                Heavy Mode executes tasks directly inside isolated rootless container environments with full CPU throughput and GPU passthrough.
              </p>
              <p style="margin-bottom: 0.75rem; color: #94a3b8; font-size: 0.85rem;">
                This one-click install sets up Apptainer locally in your user folder (<code style="color: var(--accent);">~/.local/share/speedboat/bin</code>) without requiring root/sudo privileges or terminal commands.
              </p>

              {isInstalling && (
                <div class="progress-container">
                  <div class="progress-bar">
                    <div class="progress-fill" style={{ width: `${installProgress.percent}%` }}></div>
                  </div>
                  <div class="progress-text">{installProgress.message}</div>
                </div>
              )}
            </div>
            <div class="modal-actions">
              <button
                class="btn btn-outline"
                disabled={isInstalling}
                onClick={() => {
                  setShowInstallModal(false);
                  setComputeMode('wasm');
                }}
              >
                Use Lite Mode Instead
              </button>
              <button
                class="btn btn-primary"
                disabled={isInstalling}
                onClick={handleRunInstall}
              >
                {isInstalling ? 'Installing...' : 'Install Apptainer (One-Click)'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Platform Info Modal (for non-Linux platforms) */}
      {showPlatformModal && (
        <div class="modal-backdrop">
          <div class="modal-card">
            <div class="modal-header">
              <h3>ℹ️ Heavy Mode Platform Information</h3>
            </div>
            <div class="modal-body">
              <p style="margin-bottom: 0.75rem;">
                {engineCaps?.status_message ||
                  'Heavy Mode (native rootless containers) is currently optimized for Linux environments.'}
              </p>
              <p style="color: #94a3b8; font-size: 0.85rem;">
                <strong>Lite Mode (Wazero WASM)</strong> provides high-speed, instant in-memory compute on macOS and Windows without requiring any container runtime or Docker VM.
              </p>
            </div>
            <div class="modal-actions">
              <button
                class="btn btn-primary"
                onClick={() => {
                  setShowPlatformModal(false);
                  setComputeMode('wasm');
                }}
              >
                Continue in Lite Mode
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
