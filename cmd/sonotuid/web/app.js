'use strict';

// ── State ────────────────────────────────────────────────────────────────────

const state = {
  transport: 'STOPPED',
  track: {},
  volume: 50,
  elapsed: 0,        // seconds, from last position event
  duration: 0,       // seconds
  positionTs: 0,     // performance.now() when elapsed was last set
  isPlaying: false,
  queue: [],
  speakers: [],
  activeSpeakerUUID: null,
  userScrubbing: false,
};

// ── DOM refs ─────────────────────────────────────────────────────────────────

const bgArt        = document.getElementById('bg-art');
const artImg       = document.getElementById('art');
const artPlaceholder = document.getElementById('art-placeholder');
const trackTitle   = document.getElementById('track-title');
const trackSub     = document.getElementById('track-sub');
const elapsedTime  = document.getElementById('elapsed-time');
const totalTime    = document.getElementById('total-time');
const progressFill = document.getElementById('progress-fill');
const progressInput = document.getElementById('progress-input');
const btnPrev      = document.getElementById('btn-prev');
const btnPlayPause = document.getElementById('btn-playpause');
const btnNext      = document.getElementById('btn-next');
const iconPlay     = btnPlayPause.querySelector('.icon-play');
const iconPause    = btnPlayPause.querySelector('.icon-pause');
const volumeInput  = document.getElementById('volume-input');
const queueList    = document.getElementById('queue-list');
const speakerCurrent = document.getElementById('speaker-current');
const speakerList  = document.getElementById('speaker-list');

// ── API helpers ───────────────────────────────────────────────────────────────

async function api(path, options = {}) {
  const res = await fetch(path, options);
  if (!res.ok) throw new Error(`${options.method || 'GET'} ${path}: HTTP ${res.status}`);
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

function post(path, body) {
  return api(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  });
}

function del(path) {
  return api(path, { method: 'DELETE' });
}

// ── Formatting ────────────────────────────────────────────────────────────────

function fmtTime(secs) {
  const s = Math.max(0, Math.floor(secs));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${m}:${r.toString().padStart(2, '0')}`;
}

// ── UI updates ────────────────────────────────────────────────────────────────

function applyStatus(s) {
  state.transport = s.transport ?? 'STOPPED';
  state.isPlaying = state.transport === 'PLAYING';
  state.volume    = s.volume ?? 50;
  state.elapsed   = s.elapsed ?? 0;
  state.duration  = s.duration ?? 0;
  state.positionTs = performance.now();

  applyTrack(s.track ?? {});

  volumeInput.value = state.volume;

  if (s.speaker) {
    state.activeSpeakerUUID = s.speaker.uuid;
    speakerCurrent.textContent = s.speaker.name || '—';
  }

  updateTransportUI();
  updateProgressUI();
}

function applyTrack(track) {
  const changed = track.uri !== (state.track.uri ?? '');
  state.track = track;
  state.duration = track.duration ?? state.duration;

  trackTitle.textContent = track.title || '—';
  const sub = [track.artist, track.album].filter(Boolean).join(' · ');
  trackSub.textContent = sub;

  const artUrl = track.art_url || '';
  if (changed || artImg.dataset.url !== artUrl) {
    artImg.dataset.url = artUrl;
    if (artUrl) {
      artImg.src = artUrl;
      artImg.classList.remove('hidden');
      artPlaceholder.classList.add('hidden');
      bgArt.style.backgroundImage = `url(${artUrl})`;
    } else {
      artImg.src = '';
      artImg.classList.add('hidden');
      artPlaceholder.classList.remove('hidden');
      bgArt.style.backgroundImage = '';
    }
  }
}

function updateTransportUI() {
  iconPlay.classList.toggle('hidden', state.isPlaying);
  iconPause.classList.toggle('hidden', !state.isPlaying);
}

function updateProgressUI() {
  if (state.duration > 0 && !state.userScrubbing) {
    const pct = (state.elapsed / state.duration) * 100;
    progressFill.style.width = `${pct.toFixed(2)}%`;
    progressInput.value = Math.round((state.elapsed / state.duration) * 1000);
  }
  totalTime.textContent = fmtTime(state.duration);
  if (!state.userScrubbing) {
    elapsedTime.textContent = fmtTime(state.elapsed);
  }
}

function applyQueue(items) {
  state.queue = items ?? [];
  renderQueue();
}

function renderQueue() {
  if (state.queue.length === 0) {
    queueList.innerHTML = '<div class="empty-state">Queue is empty</div>';
    return;
  }

  const currentURI = state.track.uri ?? '';
  queueList.innerHTML = '';

  for (const item of state.queue) {
    const isActive = item.uri === currentURI;
    const row = document.createElement('div');
    row.className = 'queue-item' + (isActive ? ' active' : '');
    row.innerHTML = `
      <span class="queue-pos">${item.position}</span>
      <div class="queue-meta">
        <div class="queue-title">${escHtml(item.title || '—')}</div>
        <div class="queue-artist">${escHtml(item.artist || '')}</div>
      </div>
      <span class="queue-duration">${fmtTime(item.duration ?? 0)}</span>
      <button class="queue-delete" aria-label="Remove from queue">
        <svg viewBox="0 0 24 24"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zm2.46-7.12 1.41-1.41L12 12.59l2.12-2.12 1.41 1.41L13.41 14l2.12 2.12-1.41 1.41L12 15.41l-2.12 2.12-1.41-1.41L10.59 14l-2.13-2.12zM15.5 4l-1-1h-5l-1 1H5v2h14V4z"/></svg>
      </button>`;

    row.addEventListener('click', async (e) => {
      if (e.target.closest('.queue-delete')) return;
      try { await post(`/queue/${item.position}/play`); } catch {}
    });

    row.querySelector('.queue-delete').addEventListener('click', async (e) => {
      e.stopPropagation();
      row.style.opacity = '0.3';
      try {
        await del(`/queue/${item.position}`);
      } catch {
        row.style.opacity = '';
      }
    });

    queueList.appendChild(row);
  }

  // Scroll active track into view
  const activeEl = queueList.querySelector('.queue-item.active');
  if (activeEl) activeEl.scrollIntoView({ block: 'nearest' });
}

function applySpeakers(speakers) {
  state.speakers = speakers ?? [];
  renderSpeakers();
}

function renderSpeakers() {
  speakerList.innerHTML = '';
  for (const sp of state.speakers) {
    const isActive = sp.uuid === state.activeSpeakerUUID;
    const item = document.createElement('div');
    item.className = 'speaker-item' + (isActive ? ' active' : '');
    item.innerHTML = `
      <span class="speaker-name">${escHtml(sp.name)}</span>
      <span class="speaker-check">✓</span>`;
    item.addEventListener('click', async () => {
      if (isActive) return;
      try {
        await post('/speakers/active', { uuid: sp.uuid });
        state.activeSpeakerUUID = sp.uuid;
        speakerCurrent.textContent = sp.name;
        renderSpeakers();
      } catch {}
    });
    speakerList.appendChild(item);
  }
}

function escHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// ── SSE ───────────────────────────────────────────────────────────────────────

function openSSE() {
  const es = new EventSource('/events');
  es.onmessage = (e) => {
    try { handleEvent(JSON.parse(e.data)); } catch {}
  };
  // EventSource reconnects natively on drop — no manual retry needed.
}

function handleEvent(evt) {
  switch (evt.type) {
    case 'transport':
      state.transport = evt.state ?? 'STOPPED';
      state.isPlaying = state.transport === 'PLAYING';
      updateTransportUI();
      break;

    case 'track':
      applyTrack({
        title:    evt.title,
        artist:   evt.artist,
        album:    evt.album,
        art_url:  evt.art_url,
        duration: evt.duration,
        uri:      evt.uri,
      });
      state.duration = evt.duration ?? 0;
      updateProgressUI();
      // Refresh queue to update active highlight
      api('/queue').then(applyQueue).catch(() => {});
      break;

    case 'position':
      state.elapsed   = evt.elapsed ?? 0;
      state.duration  = evt.duration ?? state.duration;
      state.positionTs = performance.now();
      if (!state.userScrubbing) updateProgressUI();
      break;

    case 'volume':
      state.volume = evt.value ?? state.volume;
      volumeInput.value = state.volume;
      break;

    case 'queue_changed':
      api('/queue').then(applyQueue).catch(() => {});
      break;

    case 'speaker':
      state.activeSpeakerUUID = evt.uuid;
      speakerCurrent.textContent = evt.name || '—';
      renderSpeakers();
      break;
  }
}

// ── Progress interpolation ────────────────────────────────────────────────────

function tick() {
  if (state.isPlaying && state.duration > 0 && !state.userScrubbing) {
    const secondsElapsed = (performance.now() - state.positionTs) / 1000;
    const current = Math.min(state.duration, state.elapsed + secondsElapsed);
    const pct = (current / state.duration) * 100;
    progressFill.style.width = `${pct.toFixed(2)}%`;
    elapsedTime.textContent = fmtTime(current);
  }
  requestAnimationFrame(tick);
}

// ── Controls ──────────────────────────────────────────────────────────────────

btnPrev.addEventListener('click', () => post('/prev').catch(() => {}));
btnNext.addEventListener('click', () => post('/next').catch(() => {}));

btnPlayPause.addEventListener('click', () => {
  const action = state.isPlaying ? '/pause' : '/play';
  post(action).catch(() => {});
});

// Progress scrubbing — we don't expose a seek endpoint, so just optimistically
// update the display while scrubbing and snap back on release.
progressInput.addEventListener('input', () => {
  state.userScrubbing = true;
  const ratio = progressInput.value / 1000;
  const pos = ratio * state.duration;
  progressFill.style.width = `${(ratio * 100).toFixed(2)}%`;
  elapsedTime.textContent = fmtTime(pos);
});

progressInput.addEventListener('change', () => {
  state.userScrubbing = false;
  updateProgressUI();
});

// Volume
let volTimer;
volumeInput.addEventListener('input', () => {
  clearTimeout(volTimer);
  volTimer = setTimeout(() => {
    post('/volume', { value: parseInt(volumeInput.value, 10) }).catch(() => {});
  }, 120);
});

// ── Tab switching ─────────────────────────────────────────────────────────────

const panels  = document.querySelectorAll('.panel');
const tabBtns = document.querySelectorAll('.tab-btn');

tabBtns.forEach(btn => {
  btn.addEventListener('click', () => {
    const target = btn.dataset.tab;
    panels.forEach(p => p.classList.toggle('hidden', p.id !== `panel-${target}`));
    tabBtns.forEach(b => b.classList.toggle('active', b === btn));
  });
});

// ── Boot ──────────────────────────────────────────────────────────────────────

async function init() {
  const [status, queue, speakers] = await Promise.all([
    api('/status'),
    api('/queue'),
    api('/speakers'),
  ]);

  applyStatus(status);
  applyQueue(queue);
  applySpeakers(speakers);
  openSSE();
  requestAnimationFrame(tick);
}

init().catch(console.error);
