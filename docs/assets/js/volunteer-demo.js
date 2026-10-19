/**
 * Project Speedboat: Volunteer Hub & Autoplaying Web Worker Simulation Demo
 * Specification: arch_docs/styles.md & arch_docs/project.txt
 */

document.addEventListener('DOMContentLoaded', () => {
  initVolunteerTabs();
  initCliGenerator();
  initSimulatedWorkerVideo();
});

/**
 * Handle volunteer portal tab switching & URL deep anchors (#desktop, #cli, #web)
 */
function initVolunteerTabs() {
  const tabBtns = document.querySelectorAll('.v-tab-btn');
  const tabPanes = document.querySelectorAll('.volunteer-tab-pane');

  function activateTab(tabId) {
    if (!tabId) return;
    tabBtns.forEach(b => {
      b.classList.toggle('active', b.getAttribute('data-tab') === tabId);
    });
    tabPanes.forEach(p => {
      p.classList.toggle('active', p.id === `pane-${tabId}`);
    });
  }

  // Handle pill button clicks on volunteer.html
  tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      const target = btn.getAttribute('data-tab');
      activateTab(target);
      history.pushState(null, null, `#${target}`);
    });
  });

  // Handle dropdown links and in-page anchor links pointing to tabs
  document.querySelectorAll('a[href*="#desktop"], a[href*="#cli"], a[href*="#web"]').forEach(link => {
    link.addEventListener('click', () => {
      const href = link.getAttribute('href') || '';
      const hash = href.includes('#') ? href.split('#')[1] : '';
      if (hash && ['desktop', 'cli', 'web'].includes(hash)) {
        activateTab(hash);
        if (window.location.pathname.includes('volunteer')) {
          history.pushState(null, null, `#${hash}`);
        }
      }
    });
  });

  // Listen for hashchange events (browser back/forward or hash updates)
  window.addEventListener('hashchange', () => {
    const hash = window.location.hash.replace('#', '');
    if (hash && ['desktop', 'cli', 'web'].includes(hash)) {
      activateTab(hash);
    }
  });

  // Activate initial tab from URL hash
  const initialHash = window.location.hash.replace('#', '');
  if (initialHash && ['desktop', 'cli', 'web'].includes(initialHash)) {
    activateTab(initialHash);
  }
}

/**
 * CLI Command Builder
 */
function initCliGenerator() {
  const projectInput = document.getElementById('cli-project');
  const natsInput = document.getElementById('cli-nats');
  const runtimeSelect = document.getElementById('cli-runtime');
  const idleInput = document.getElementById('cli-idle');
  const outputPre = document.getElementById('cli-command-output');

  if (!projectInput || !natsInput || !outputPre) return;

  function updateCommand() {
    const p = projectInput.value.trim() || 'prime-counter';
    const n = natsInput.value.trim() || 'nats://localhost:4222';
    const r = runtimeSelect.value;
    const idle = idleInput.value.trim();

    let cmd = `speedboat-agent run --project-id ${p} --nats-url ${n}`;
    if (r !== 'auto') {
      cmd += ` --runtime ${r}`;
    }
    if (idle && idle !== '30') {
      cmd += ` --idle-timeout ${idle}`;
    }

    outputPre.innerText = cmd;
  }

  [projectInput, natsInput, runtimeSelect, idleInput].forEach(el => {
    if (el) el.addEventListener('input', updateCommand);
    if (el) el.addEventListener('change', updateCommand);
  });

  updateCommand();
}

/**
 * Video-Style Autoplaying Web Worker Simulation Demo
 * Shows the exact automated volunteer experience when opening a project URL
 */
function initSimulatedWorkerVideo() {
  const playBtn = document.getElementById('sim-play-btn');
  const restartBtn = document.getElementById('sim-restart-btn');
  const timelineFill = document.getElementById('sim-timeline-fill');
  const timeText = document.getElementById('sim-time-text');

  const valTasks = document.getElementById('sim-tasks');
  const valTime = document.getElementById('sim-time');
  const valCredits = document.getElementById('sim-credits');
  const valLatency = document.getElementById('sim-latency');
  const progressFill = document.getElementById('sim-progress');
  const rankBadgeTitle = document.getElementById('sim-rank-title');
  const rankBadgeIcon = document.getElementById('sim-rank-icon');
  const rankToast = document.getElementById('rank-toast');
  const rankToastMsg = document.getElementById('rank-toast-msg');
  const terminalBody = document.getElementById('sim-terminal-body');
  const browserStatus = document.getElementById('sim-browser-status');

  if (!timelineFill) return;

  const totalDurationSec = 30;
  let elapsedSec = 0;
  let isPlaying = true;
  let animationInterval = null;

  let tasksCompleted = 0;
  let totalComputeMs = 0;
  let currentRank = 'Cadet Pilot';

  const ranks = [
    { threshold: 0, title: 'Cadet Pilot', icon: '⚡' },
    { threshold: 5, title: 'Hydrofoil Specialist', icon: '🌊' },
    { threshold: 20, title: 'Supersonic Voyager', icon: '🚀' }
  ];

  function formatTime(s) {
    const mins = Math.floor(s / 60);
    const secs = Math.floor(s % 60);
    return `${mins}:${secs < 10 ? '0' : ''}${secs}`;
  }

  function appendLog(level, msg) {
    if (!terminalBody) return;
    const now = new Date().toTimeString().split(' ')[0];
    const line = document.createElement('div');
    line.className = 'terminal-line';
    line.innerHTML = `
      <span class="terminal-time">[${now}]</span>
      <span class="terminal-level ${level}">[${level.toUpperCase()}]</span>
      <span class="terminal-msg">${msg}</span>
    `;
    terminalBody.appendChild(line);
    terminalBody.scrollTop = terminalBody.scrollHeight;
  }

  function resetSimulation() {
    elapsedSec = 0;
    tasksCompleted = 0;
    totalComputeMs = 0;
    currentRank = 'Cadet Pilot';

    if (valTasks) valTasks.innerText = '0';
    if (valTime) valTime.innerText = '0 ms';
    if (valCredits) valCredits.innerText = '0';
    if (valLatency) valLatency.innerText = '0 ms';
    if (progressFill) progressFill.style.width = '10%';
    if (rankBadgeTitle) rankBadgeTitle.innerText = 'Cadet Pilot';
    if (rankBadgeIcon) rankBadgeIcon.innerText = '⚡';
    if (timelineFill) timelineFill.style.width = '0%';
    if (timeText) timeText.innerText = `0:00 / ${formatTime(totalDurationSec)}`;
    if (terminalBody) terminalBody.innerHTML = '';
    if (browserStatus) {
      browserStatus.innerHTML = '<span class="status-dot active"></span> <span>INITIALIZING...</span>';
    }

    appendLog('info', 'Volunteer opened project URL https://grid.speedboat.dev/p/prime-counter in browser.');
    appendLog('info', 'Web Worker thread spawned. Negotiating nats.ws WebSocket handshake...');
  }

  function tick() {
    if (!isPlaying) return;

    elapsedSec += 0.5;

    // Timeline progress update
    const percent = Math.min(100, (elapsedSec / totalDurationSec) * 100);
    if (timelineFill) timelineFill.style.width = `${percent}%`;
    if (timeText) timeText.innerText = `${formatTime(elapsedSec)} / ${formatTime(totalDurationSec)}`;

    // Scripted simulation milestones
    if (elapsedSec === 1.5) {
      if (browserStatus) {
        browserStatus.innerHTML = '<span class="status-dot emerald"></span> <span>CONNECTED (nats.ws)</span>';
      }
      appendLog('success', 'Connected to NATS message broker. Downloading logic.wasm (1.2 MB)...');
    } else if (elapsedSec === 3.0) {
      appendLog('success', 'WebAssembly JIT initialized in memory. Listening for task work queue...');
    } else if (elapsedSec >= 4.0) {
      // Simulate rapid batch compute step
      const taskBatch = Math.floor(Math.random() * 2) + 1;
      for (let i = 0; i < taskBatch; i++) {
        tasksCompleted++;
        const stepMs = 12 + Math.floor(Math.random() * 14);
        totalComputeMs += stepMs;
        const credits = (tasksCompleted * 100) + Math.floor(totalComputeMs / 100);

        if (valTasks) valTasks.innerText = tasksCompleted.toLocaleString();
        if (valTime) valTime.innerText = `${totalComputeMs.toLocaleString()} ms`;
        if (valCredits) valCredits.innerText = credits.toLocaleString();
        if (valLatency) valLatency.innerText = `${stepMs} ms`;

        // Rank updates
        let newRank = ranks[0];
        for (let r = 0; r < ranks.length; r++) {
          if (tasksCompleted >= ranks[r].threshold) newRank = ranks[r];
        }

        if (newRank.title !== currentRank) {
          currentRank = newRank.title;
          if (rankBadgeTitle) rankBadgeTitle.innerText = currentRank;
          if (rankBadgeIcon) rankBadgeIcon.innerText = newRank.icon;

          if (rankToast && rankToastMsg) {
            rankToastMsg.innerText = `Promoted to ${currentRank}!`;
            rankToast.classList.add('show');
            setTimeout(() => rankToast.classList.remove('show'), 3500);
          }
        }

        const progPct = Math.min(100, (tasksCompleted / 20) * 100);
        if (progressFill) progressFill.style.width = `${Math.max(10, progPct)}%`;

        if (tasksCompleted % 3 === 0) {
          const hash = Math.random().toString(16).substring(2, 8);
          appendLog('info', `Task #${tasksCompleted} calculated. Majority consensus 3/3 verified. Hash: <code>${hash}</code>`);
        }
      }
    }

    // Loop after duration
    if (elapsedSec >= totalDurationSec) {
      appendLog('success', 'Demo simulation cycle complete. Seamlessly looping video preview...');
      resetSimulation();
    }
  }

  // Play / Pause toggle
  if (playBtn) {
    playBtn.addEventListener('click', () => {
      isPlaying = !isPlaying;
      playBtn.innerHTML = isPlaying ? '⏸' : '▶';
      playBtn.setAttribute('title', isPlaying ? 'Pause Demo' : 'Play Demo');
    });
  }

  // Restart button
  if (restartBtn) {
    restartBtn.addEventListener('click', () => {
      resetSimulation();
      if (!isPlaying) {
        isPlaying = true;
        if (playBtn) playBtn.innerHTML = '⏸';
      }
    });
  }

  // Start auto-playback
  resetSimulation();
  animationInterval = setInterval(tick, 500);
}
