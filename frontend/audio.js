// Per-video sound: volume, mute and "Hear it in Studio". New videos start
// unmuted at 3%. Changes made while LIVE are sent to the running stream
// without restarting it.
"use strict";

const DEFAULT_VIDEO_VOLUME = 0.03;
const hasSound = (s) => s.kind === "video" && !!s.use_audio;
// videos saved before per-video volume existed play at 100%
const srcVolume = (s) => (s.volume == null ? 1 : +s.volume);
const pct = (v) => Math.round(v * 100) + "%";

function audioIcon(s) {
  return s.muted || srcVolume(s) === 0 ? "mute" : "volume";
}

// Send the levels to the running stream (debounced for slider drags).
const applyAudioLive = debounce(async () => {
  const id = S.acctId;
  if (!isLive(id)) return;
  try {
    await api("POST", acctPath("/encoder/audio", id), { config: sceneConfig() });
  } catch (e) {
    // e.g. the scene was edited and not applied yet; Apply to LIVE covers it
    St.dirtyLive = true;
    drawLiveBits();
    toast(e.message, "err");
  }
}, 120);

function setSourceAudio(s, patch) {
  Object.assign(s, patch);
  saveScene();
  drawSources();
  syncHear();
  applyAudioLive();
}

// Speaker popover for a video in the sources list.
function openAudioPop(anchor, s) {
  closeMenus();
  const pop = node(
    '<div class="menu audio-pop">' +
      '<div class="ap-head"><b>' +
      esc(sourceTitle(s)) +
      '</b><span class="hint">Video sound on your LIVE</span></div>' +
      '<div class="ap-row"><button class="icon-btn" id="ap-mute" title="Mute">' +
      icon(audioIcon(s)) +
      "</button>" +
      '<input type="range" class="range" id="ap-vol" min="0" max="1" step="0.01"><b class="ap-pct" id="ap-pct"></b></div>' +
      '<div class="ap-presets">' +
      [0.03, 0.1, 0.25, 0.5, 1].map((v) => '<button class="chip" data-v="' + v + '">' + pct(v) + "</button>").join("") +
      "</div>" +
      '<label class="ap-hear"><input type="checkbox" id="ap-hear"' +
      (s.hear ? " checked" : "") +
      '> Hear it in Studio <span class="hint">(only you)</span></label></div>',
  );
  document.body.appendChild(pop);
  const r = anchor.getBoundingClientRect();
  pop.style.left = Math.max(8, Math.min(r.left, innerWidth - pop.offsetWidth - 8)) + "px";
  pop.style.top = Math.min(r.bottom + 6, innerHeight - pop.offsetHeight - 8) + "px";
  const range = $("#ap-vol", pop),
    label = $("#ap-pct", pop),
    mute = $("#ap-mute", pop);
  const draw = () => {
    range.value = srcVolume(s);
    paintRange(range);
    label.textContent = s.muted ? "Muted" : pct(srcVolume(s));
    label.classList.toggle("off", !!s.muted);
    mute.innerHTML = icon(audioIcon(s));
    mute.title = s.muted ? "Unmute" : "Mute";
    $$(".chip", pop).forEach((c) =>
      c.classList.toggle("on", !s.muted && Math.abs(+c.dataset.v - srcVolume(s)) < 0.005),
    );
  };
  range.oninput = () => {
    setSourceAudio(s, { volume: +range.value, muted: false });
    draw();
  };
  mute.onclick = () => {
    setSourceAudio(s, { muted: !s.muted });
    draw();
  };
  $$(".chip", pop).forEach(
    (c) =>
      (c.onclick = () => {
        setSourceAudio(s, { volume: +c.dataset.v, muted: false });
        draw();
      }),
  );
  $("#ap-hear", pop).onchange = (e) => setSourceAudio(s, { hear: e.target.checked });
  draw();
  const off = (e) => {
    if (!pop.contains(e.target) && e.target !== anchor && !anchor.contains(e.target)) closeMenus();
  };
  setTimeout(() => document.addEventListener("mousedown", off), 0);
  pop.off = () => document.removeEventListener("mousedown", off);
}

// "Hear it in Studio" plays the video file locally, looped, at the level
// viewers get (capped at 100%, the most an <audio> element can do).
const HEAR = {};
function syncHear() {
  const want = new Set();
  if (S.view === "studio") {
    for (const s of visibleSources()) {
      if (!hasSound(s) || !s.hear) continue;
      want.add(s.id);
      let a = HEAR[s.id];
      if (!a || a.dataset.path !== s.path) {
        if (a) a.pause();
        a = new Audio("/api/media?path=" + encodeURIComponent(s.path));
        a.loop = true;
        a.dataset.path = s.path;
        a.onerror = () => {
          toast(
            "Can't play this video's sound in Studio (" +
              (s.path.split(".").pop() || "format") +
              "). Your LIVE still gets it.",
            "err",
          );
          setSourceAudio(s, { hear: false });
        };
        HEAR[s.id] = a;
      }
      a.volume = Math.min(1, s.muted ? 0 : srcVolume(s) * (+E.vidVol || 0));
      if (a.paused) a.play().catch(() => {});
    }
  }
  for (const id of Object.keys(HEAR))
    if (!want.has(id)) {
      HEAR[id].pause();
      HEAR[id].removeAttribute("src");
      delete HEAR[id];
    }
}
function stopHear() {
  const v = S.view;
  S.view = "";
  syncHear();
  S.view = v;
}
