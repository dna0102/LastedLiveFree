// Studio canvas when not LIVE. Each source is its own layer: images come
// straight from disk, videos/screens/windows stream from a small preview per
// source, and positioning, cropping and flipping is done with CSS. That keeps
// editing instant; ffmpeg only restarts when a source is added, removed or
// swapped. While LIVE the canvas shows the actual stream instead.
"use strict";

const LY = { loop: 0, seq: {}, url: {}, synced: "", errs: {} };
const STREAMED = (s) => s.kind === "video" || s.kind === "screen";

// Key for what a source shows, independent of where it's placed. Same key,
// same preview stream.
function layerKey(s) {
  const id =
    s.kind === "video"
      ? "v|" + s.path
      : s.kind === "screen"
        ? s.window
          ? "w|" + s.window + "|" + (s.window_app || "")
          : "m|" + (s.monitor | 0) + "|" + (s.cursor ? 1 : 0)
        : "";
  let h = 5381;
  for (let i = 0; i < id.length; i++) h = ((h << 5) + h + id.charCodeAt(i)) >>> 0;
  return "L" + h.toString(16);
}
const editorMode = () => !isLive();

function layerSig(s) {
  return s.kind + "|" + (s.kind === "image" ? s.path : STREAMED(s) ? layerKey(s) : "");
}
function makeLayer(s) {
  const el = node('<div class="layer k-' + s.kind + '" data-id="' + esc(s.id) + '"><div class="lw"></div></div>');
  const lw = el.firstElementChild;
  if (s.kind === "image")
    lw.appendChild(node('<img class="lc" draggable="false" src="/api/file?path=' + encodeURIComponent(s.path) + '">'));
  else if (STREAMED(s)) lw.appendChild(node('<img class="lc" draggable="false">'));
  else if (s.kind === "color") lw.appendChild(node('<div class="lcolor"></div>'));
  else if (s.kind === "text") lw.appendChild(node('<div class="ltext"></div>'));
  el.dataset.sig = layerSig(s);
  return el;
}

// Position a layer for s, which may be a drag in progress rather than the saved
// source.
function positionLayer(el, s) {
  if (!el) return;
  const k = St.scale || 1;
  const lw = el.firstElementChild,
    c = lw.firstElementChild;
  if (s.kind === "text") {
    Object.assign(el.style, { left: (s.x | 0) * k + "px", top: (s.y | 0) * k + "px", width: "auto", height: "auto" });
    c.textContent = s.text || "";
    Object.assign(c.style, {
      fontSize: (s.font_size || 72) * k + "px",
      color: s.color || "white",
      webkitTextStroke: Math.max(0.5, 3 * k) + "px rgba(0,0,0,.55)",
    });
    return;
  }
  const b = boxFor(s);
  Object.assign(el.style, { left: b.x * k + "px", top: b.y * k + "px", width: b.w * k + "px", height: b.h * k + "px" });
  lw.style.transform = s.flip_h || s.flip_v ? "scale(" + (s.flip_h ? -1 : 1) + "," + (s.flip_v ? -1 : 1) + ")" : "";
  if (s.kind === "color") {
    c.style.background = s.color || "white";
    return;
  }
  if (s.nat_w && s.nat_h) {
    const [cw, ch] = cropped(s),
      sx = b.w / cw,
      sy = b.h / ch;
    Object.assign(c.style, {
      width: s.nat_w * sx * k + "px",
      height: s.nat_h * sy * k + "px",
      left: -(s.crop_l | 0) * sx * k + "px",
      top: -(s.crop_t | 0) * sy * k + "px",
    });
  } else Object.assign(c.style, { width: "100%", height: "100%", left: "0", top: "0" });
}
const layerEl = (id) => $('#st-layers .layer[data-id="' + CSS.escape(id) + '"]');

function drawLayers() {
  const host = $("#st-layers");
  if (!host) return;
  const on = editorMode();
  host.hidden = !on;
  const frame = $("#pv-frame");
  if (frame && on) frame.hidden = true;
  if (!on) return;
  const keep = new Set();
  for (const s of visibleSources()) {
    let el = layerEl(s.id);
    if (el && el.dataset.sig !== layerSig(s)) {
      el.remove();
      el = null;
    }
    if (!el) el = makeLayer(s);
    positionLayer(el, s);
    el.classList.toggle("missing", STREAMED(s) && !!LY.errs[layerKey(s)]);
    host.appendChild(el); // re-appending in scene order keeps the z-order right
    keep.add(s.id);
  }
  $$(".layer", host).forEach((el) => {
    if (!keep.has(el.dataset.id)) el.remove();
  });
}

function layerConfig(s) {
  return {
    kind: s.kind,
    path: s.path || "",
    monitor: s.monitor | 0,
    cursor: !!s.cursor,
    window: s.window || "",
    window_app: s.window_app || "",
    window_title: s.window_title || "",
    x: 0,
    y: 0,
    w: 0,
    h: 0,
    use_audio: false,
  };
}
async function syncLayers() {
  const id = S.acctId;
  const want = {};
  if (S.view === "studio" && editorMode())
    for (const s of visibleSources()) if (STREAMED(s)) want[layerKey(s)] = layerConfig(s);
  const sig = id + ":" + Object.keys(want).sort().join(",");
  if (sig === LY.synced) return;
  LY.synced = sig;
  try {
    const d = await api("POST", acctPath("/layers", id), { layers: want });
    LY.errs = d.errors || {};
    if (Object.keys(LY.errs).length) drawLayers();
  } catch {}
}
const syncLayersSoon = debounce(syncLayers, 150);
function stopLayers(id = S.acctId) {
  LY.synced = "";
  LY.loop++;
  api("POST", acctPath("/layers", id), { layers: {} }).catch(() => {});
}

async function layerLoop() {
  const token = ++LY.loop;
  while (token === LY.loop) {
    let got = false;
    if (editorMode() && !document.hidden) {
      const id = S.acctId;
      const tasks = visibleSources()
        .filter(STREAMED)
        .map(async (s) => {
          const key = layerKey(s);
          try {
            const r = await fetch(acctPath("/layers/" + key + "/frame?since=" + (LY.seq[key] || 0), id), {
              cache: "no-store",
            });
            if (r.status !== 200 || token !== LY.loop) return;
            LY.seq[key] = +r.headers.get("x-frame-seq") || (LY.seq[key] || 0) + 1;
            const url = URL.createObjectURL(await r.blob());
            const img = $('#st-layers .layer[data-id="' + CSS.escape(s.id) + '"] img');
            if (img) img.src = url;
            if (LY.url[s.id]) URL.revokeObjectURL(LY.url[s.id]);
            LY.url[s.id] = url;
            got = true;
          } catch {}
        });
      await Promise.all(tasks);
    }
    await sleep(got ? 12 : document.hidden ? 500 : 40);
  }
}

function layersStart() {
  LY.seq = {};
  LY.synced = "";
  drawLayers();
  syncLayers();
  layerLoop();
  prepareScene();
}
function layersStop() {
  stopLayers();
  Object.values(LY.url).forEach((u) => URL.revokeObjectURL(u));
  LY.url = {};
}

// Look up each media source's size and convert old fit/fill layers to exact
// boxes, so the canvas matches the stream.
async function prepareScene() {
  const acctAtStart = S.acctId;
  let changed = false;
  for (const s of scene().sources) {
    if (!XF_KINDS.includes(s.kind) || s.kind === "color") continue;
    const had = !!s.nat_w;
    if (!(await ensureNat(s))) continue;
    if (S.acctId !== acctAtStart) return;
    if (legacyFit(s) !== "stretch") {
      normalize(s);
      changed = true;
    } else if (!had) changed = true;
  }
  if (changed) {
    saveScene();
    drawLayers();
    renderSelBox();
  }
}
