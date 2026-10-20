/**
 * Project Speedboat: Hydrofoil HyperGrid Global Scripts
 * Specification: arch_docs/styles.md & arch_docs/project.txt
 */

document.addEventListener('DOMContentLoaded', () => {
  initWakeCanvas();
  initTopbarScroll();
  initMobileMenu();
  initCopyButtons();
  initDownloadModal();
  initArchitectureExplorer();
  initOSDetection();
  initReleaseVersionBadge();
});

/**
 * Animated Hydrofoil Boat & Laminar V-Wake Canvas
 */
function initWakeCanvas() {
  const canvas = document.getElementById('wake-canvas');
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  let width = (canvas.width = window.innerWidth);
  let height = (canvas.height = window.innerHeight);

  window.addEventListener('resize', () => {
    width = canvas.width = window.innerWidth;
    height = canvas.height = window.innerHeight;
  });

  const boat = {
    x: width / 2,
    y: Math.min(130, height * 0.16),
    targetX: width / 2,
    tilt: 0
  };

  window.addEventListener('mousemove', (e) => {
    boat.targetX = e.clientX;
  }, { passive: true });

  const wakeWaves = [];
  const streamParticles = [];
  let frame = 0;

  function spawnWave() {
    wakeWaves.push({
      cx: boat.x,
      cy: boat.y + 16,
      spread: 0,
      alpha: 0.38,
      speed: 2.2,
      maxSpread: Math.max(width * 0.65, 500)
    });
  }

  function spawnParticles() {
    for (let i = 0; i < 2; i++) {
      const isLeft = i === 0;
      streamParticles.push({
        x: boat.x + (isLeft ? -18 : 18) + (Math.random() * 6 - 3),
        y: boat.y + 20,
        vx: (isLeft ? -1 : 1) * (1.2 + Math.random() * 2.2),
        vy: 2.2 + Math.random() * 2.8,
        length: 8 + Math.random() * 18,
        alpha: 0.4 + Math.random() * 0.3,
        color: Math.random() > 0.3 ? '0, 240, 255' : '168, 85, 247'
      });
    }
  }

  function render() {
    ctx.clearRect(0, 0, width, height);
    frame++;

    // Ease boat position toward mouse
    boat.x += (boat.targetX - boat.x) * 0.04;
    boat.tilt = (boat.targetX - boat.x) * 0.0012;
    boat.y = Math.min(130, height * 0.16);

    // Spawning
    if (frame % 8 === 0) spawnWave();
    if (frame % 3 === 0) spawnParticles();

    // 1. Draw Expanding V-Wake Waves (Boat cutting through water)
    for (let i = wakeWaves.length - 1; i >= 0; i--) {
      const w = wakeWaves[i];
      w.cy += w.speed;
      w.spread += w.speed * 1.8;
      w.alpha *= 0.988;

      if (w.alpha < 0.01 || w.spread > w.maxSpread) {
        wakeWaves.splice(i, 1);
        continue;
      }

      ctx.save();
      ctx.beginPath();
      // Left V-wake arm
      ctx.moveTo(w.cx, w.cy);
      ctx.lineTo(w.cx - w.spread, w.cy + w.spread * 0.55);
      // Right V-wake arm
      ctx.moveTo(w.cx, w.cy);
      ctx.lineTo(w.cx + w.spread, w.cy + w.spread * 0.55);

      ctx.strokeStyle = `rgba(0, 240, 255, ${w.alpha})`;
      ctx.lineWidth = 1.2;
      ctx.shadowColor = '#00f0ff';
      ctx.shadowBlur = 6;
      ctx.stroke();
      ctx.restore();
    }

    // 2. Draw Water Streamline Particles
    for (let i = streamParticles.length - 1; i >= 0; i--) {
      const p = streamParticles[i];
      p.x += p.vx;
      p.y += p.vy;
      p.alpha *= 0.978;

      if (p.alpha < 0.02 || p.y > height) {
        streamParticles.splice(i, 1);
        continue;
      }

      ctx.save();
      ctx.beginPath();
      ctx.moveTo(p.x, p.y);
      ctx.lineTo(p.x + p.vx * 2, p.y + p.length);
      ctx.strokeStyle = `rgba(${p.color}, ${p.alpha})`;
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    }

    // 3. Draw Continuous Subtle Wake Rays (Bow spray lines)
    ctx.save();
    const rayAlpha = 0.12;
    const gradLeft = ctx.createLinearGradient(boat.x - 14, boat.y + 14, 0, height);
    gradLeft.addColorStop(0, `rgba(0, 240, 255, ${rayAlpha})`);
    gradLeft.addColorStop(1, 'rgba(0, 240, 255, 0)');
    ctx.beginPath();
    ctx.moveTo(boat.x - 14, boat.y + 14);
    ctx.lineTo(Math.max(0, boat.x - width * 0.6), height);
    ctx.strokeStyle = gradLeft;
    ctx.lineWidth = 1.5;
    ctx.stroke();

    const gradRight = ctx.createLinearGradient(boat.x + 14, boat.y + 14, width, height);
    gradRight.addColorStop(0, `rgba(0, 240, 255, ${rayAlpha})`);
    gradRight.addColorStop(1, 'rgba(0, 240, 255, 0)');
    ctx.beginPath();
    ctx.moveTo(boat.x + 14, boat.y + 14);
    ctx.lineTo(Math.min(width, boat.x + width * 0.6), height);
    ctx.strokeStyle = gradRight;
    ctx.lineWidth = 1.5;
    ctx.stroke();
    ctx.restore();

    // 4. Draw Minimalist Hydrofoil Speedboat Hull
    ctx.save();
    ctx.translate(boat.x, boat.y);
    ctx.rotate(boat.tilt);

    // Glowing Thruster Flare
    const thrusterGrad = ctx.createLinearGradient(0, 14, 0, 36);
    thrusterGrad.addColorStop(0, 'rgba(0, 240, 255, 0.8)');
    thrusterGrad.addColorStop(0.6, 'rgba(168, 85, 247, 0.4)');
    thrusterGrad.addColorStop(1, 'rgba(168, 85, 247, 0)');
    ctx.beginPath();
    ctx.moveTo(-4, 16);
    ctx.lineTo(0, 32);
    ctx.lineTo(4, 16);
    ctx.fillStyle = thrusterGrad;
    ctx.fill();

    // Lateral Hydrofoils
    ctx.beginPath();
    ctx.moveTo(-16, 14);
    ctx.lineTo(-28, 22);
    ctx.moveTo(16, 14);
    ctx.lineTo(28, 22);
    ctx.strokeStyle = 'rgba(0, 240, 255, 0.6)';
    ctx.lineWidth = 1.5;
    ctx.stroke();

    // Supersonic Hull Wedge
    ctx.beginPath();
    ctx.moveTo(0, -22); // Prow apex
    ctx.lineTo(-15, 16); // Left stern
    ctx.lineTo(0, 11);   // Keel notch
    ctx.lineTo(15, 16);  // Right stern
    ctx.closePath();
    ctx.fillStyle = 'rgba(10, 16, 30, 0.85)';
    ctx.fill();
    ctx.strokeStyle = '#00f0ff';
    ctx.lineWidth = 1.4;
    ctx.shadowColor = '#00f0ff';
    ctx.shadowBlur = 10;
    ctx.stroke();

    // Inner Cockpit Glow
    ctx.beginPath();
    ctx.moveTo(0, -12);
    ctx.lineTo(-6, 4);
    ctx.lineTo(0, 2);
    ctx.lineTo(6, 4);
    ctx.closePath();
    ctx.fillStyle = 'rgba(0, 240, 255, 0.75)';
    ctx.fill();

    ctx.restore();

    requestAnimationFrame(render);
  }

  render();
}

/**
 * Topbar Scroll Effect
 */
function initTopbarScroll() {
  const topbar = document.querySelector('.topbar');
  if (!topbar) return;

  const onScroll = () => {
    if (window.scrollY > 20) {
      topbar.classList.add('scrolled');
    } else {
      topbar.classList.remove('scrolled');
    }
  };

  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();
}

/**
 * Mobile Hamburger Menu
 */
function initMobileMenu() {
  const toggle = document.querySelector('.mobile-toggle');
  const menu = document.querySelector('.mobile-menu');
  if (!toggle || !menu) return;

  toggle.addEventListener('click', () => {
    const isOpen = menu.classList.toggle('open');
    toggle.innerHTML = isOpen ? '✕' : '☰';
  });

  menu.querySelectorAll('a').forEach(link => {
    link.addEventListener('click', () => {
      menu.classList.remove('open');
      toggle.innerHTML = '☰';
    });
  });
}

/**
 * Copy to Clipboard Buttons
 */
function initCopyButtons() {
  document.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const targetId = btn.getAttribute('data-target');
      let text = '';

      if (targetId) {
        const el = document.getElementById(targetId);
        text = el ? el.innerText : '';
      } else {
        const pre = btn.closest('.code-block-wrapper')?.querySelector('pre');
        text = pre ? pre.innerText : '';
      }

      if (!text) return;

      try {
        await navigator.clipboard.writeText(text);
        const originalHtml = btn.innerHTML;
        btn.innerHTML = '✓ Copied!';
        btn.style.color = 'var(--quantum-emerald)';
        btn.style.borderColor = 'rgba(16, 185, 129, 0.4)';

        setTimeout(() => {
          btn.innerHTML = originalHtml;
          btn.style.color = '';
          btn.style.borderColor = '';
        }, 2000);
      } catch (err) {
        console.error('Failed to copy', err);
      }
    });
  });
}

/**
 * Download Modal Logic
 */
function initDownloadModal() {
  const overlay = document.getElementById('download-modal');
  if (!overlay) return;

  const openBtns = document.querySelectorAll('[data-open-download]');
  const closeBtn = overlay.querySelector('.modal-close');

  const openModal = (e) => {
    if (e) e.preventDefault();
    overlay.classList.add('open');
    document.body.style.overflow = 'hidden';
  };

  const closeModal = () => {
    overlay.classList.remove('open');
    document.body.style.overflow = '';
  };

  openBtns.forEach(btn => btn.addEventListener('click', openModal));
  if (closeBtn) closeBtn.addEventListener('click', closeModal);

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) closeModal();
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && overlay.classList.contains('open')) {
      closeModal();
    }
  });
}

/**
 * Architecture Diagram Node Selector
 */
function initArchitectureExplorer() {
  const nodes = document.querySelectorAll('.arch-node');
  const detailTitle = document.getElementById('arch-detail-title');
  const detailDesc = document.getElementById('arch-detail-desc');
  const detailBadge = document.getElementById('arch-detail-badge');

  if (!nodes.length || !detailTitle || !detailDesc) return;

  const nodeInfo = {
    master: {
      title: 'Master Node (Orchestrator)',
      badge: 'Orchestration Engine',
      desc: 'Dispatches NATS work queues, automates container image baking, cross-compiles WebAssembly, and validates results using 3-way consensus.'
    },
    nats: {
      title: 'NATS.io Message Bus & Object Store',
      badge: 'Real-Time Fabric',
      desc: 'Punches through enterprise firewalls and NATs using native WebSocket and TCP connections. Distributes .wasm payloads via JetStream Object Store.'
    },
    desktop: {
      title: 'Primary Worker (Native Go agent.exe)',
      badge: 'Single-Binary Agent',
      desc: 'Zero-friction 15MB client with zero external dependencies. Dynamically flips between Heavy Mode (rootless crun/GPU) and Lite Mode (embedded wazero JIT).'
    },
    browser: {
      title: 'Secondary Worker (Browser WASM)',
      badge: 'Zero-Install Fallback',
      desc: 'Runs directly inside standard browser tabs using Web Workers and nats.ws. Volunteers simply open a project link to immediately start computing.'
    }
  };

  nodes.forEach(node => {
    node.addEventListener('click', () => {
      nodes.forEach(n => n.classList.remove('active-node'));
      node.classList.add('active-node');

      const key = node.getAttribute('data-node');
      const info = nodeInfo[key];
      if (info) {
        detailTitle.innerText = info.title;
        detailDesc.innerText = info.desc;
        if (detailBadge) detailBadge.innerText = info.badge;
      }
    });
  });
}

/**
 * Dynamic Operating System Detection & Highlighting
 * Highlights client release cards and download modal options matching the visitor's OS
 */
function initOSDetection() {
  const ua = (navigator.userAgent || '').toLowerCase();
  const platform = (navigator.userAgentData?.platform || navigator.platform || '').toLowerCase();

  let detectedOS = 'linux'; // Default to linux if detection is ambiguous or on Linux
  if (platform.includes('win') || ua.includes('windows') || ua.includes('win64') || ua.includes('win32')) {
    detectedOS = 'windows';
  } else if (platform.includes('mac') || ua.includes('macintosh') || ua.includes('mac os x') || ua.includes('darwin')) {
    detectedOS = 'mac';
  } else if (platform.includes('linux') || ua.includes('linux') || ua.includes('x11')) {
    detectedOS = 'linux';
  }

  const osItems = document.querySelectorAll('[data-os]');
  if (!osItems.length) return;

  osItems.forEach(item => {
    const itemOS = item.getAttribute('data-os');
    const isMatch = itemOS === detectedOS;
    const btns = item.querySelectorAll('.btn');
    const tag = item.querySelector('.dl-tag');

    if (isMatch) {
      item.classList.add('detected-os');
      btns.forEach(btn => {
        btn.classList.remove('btn-outline');
        btn.classList.add('btn-solid-cyan');
      });
      if (tag) {
        if (!tag.getAttribute('data-original-text')) {
          tag.setAttribute('data-original-text', tag.textContent.trim());
        }
        const orig = tag.getAttribute('data-original-text');
        if (!orig.includes('(Your OS)')) {
          tag.textContent = `${orig} (Your OS)`;
        }
      }
    } else {
      item.classList.remove('detected-os');
      btns.forEach(btn => {
        btn.classList.remove('btn-solid-cyan');
        btn.classList.add('btn-outline');
      });
      if (tag && tag.getAttribute('data-original-text')) {
        tag.textContent = tag.getAttribute('data-original-text');
      }
    }
  });
}

/**
 * Dynamic Release Tag Fetcher
 * Reads latest release from GitHub API and updates data-release-version placeholders
 */
function initReleaseVersionBadge() {
  const versionTags = document.querySelectorAll('[data-release-version]');
  if (!versionTags.length) return;

  fetch('https://api.github.com/repos/StanaAndrei/speedboat/releases/latest')
    .then(res => {
      if (!res.ok) throw new Error('Release query returned non-200');
      return res.json();
    })
    .then(data => {
      if (data && data.tag_name) {
        versionTags.forEach(el => {
          el.textContent = data.tag_name;
        });
      }
    })
    .catch(() => {
      // Graceful fallback to initial static text
    });
}

// Immediate run in case DOM is already ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    initOSDetection();
    initReleaseVersionBadge();
  });
} else {
  initOSDetection();
  initReleaseVersionBadge();
}
