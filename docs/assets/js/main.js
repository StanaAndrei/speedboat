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
});

/**
 * Animated Hydrofoil Wake & Laminar Stream Canvas
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

  // Wake stream particles
  const particleCount = Math.min(50, Math.floor(width / 25));
  const particles = [];

  for (let i = 0; i < particleCount; i++) {
    particles.push({
      x: Math.random() * width,
      y: Math.random() * height,
      length: 40 + Math.random() * 80,
      speed: 1.2 + Math.random() * 2.5,
      opacity: 0.15 + Math.random() * 0.35,
      color: Math.random() > 0.4 ? '0, 240, 255' : '168, 85, 247',
      width: Math.random() > 0.7 ? 1.5 : 0.8
    });
  }

  function render() {
    ctx.clearRect(0, 0, width, height);

    // Draw laminar streamline wakes
    for (let i = 0; i < particles.length; i++) {
      const p = particles[i];
      p.y -= p.speed;
      p.x += Math.sin(p.y * 0.005) * 0.4;

      if (p.y + p.length < 0) {
        p.y = height + Math.random() * 50;
        p.x = Math.random() * width;
      }

      const grad = ctx.createLinearGradient(p.x, p.y, p.x, p.y + p.length);
      grad.addColorStop(0, `rgba(${p.color}, 0)`);
      grad.addColorStop(0.5, `rgba(${p.color}, ${p.opacity})`);
      grad.addColorStop(1, `rgba(${p.color}, 0)`);

      ctx.beginPath();
      ctx.moveTo(p.x, p.y);
      ctx.lineTo(p.x, p.y + p.length);
      ctx.strokeStyle = grad;
      ctx.lineWidth = p.width;
      ctx.stroke();
    }

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
