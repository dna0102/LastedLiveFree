// Customise tab: encoder, audio, saved scenes and system info.
"use strict";

const Cu = { tab: LS.get("custTab", "encoder"), timer: 0 };
const CU_TABS = [
  { id: "encoder", label: "Encoder", icon: "video" },
  { id: "audio", label: "Audio", icon: "audio" },
  { id: "scenes", label: "Scenes", icon: "layers" },
  { id: "system", label: "System", icon: "monitor" },
];

VIEWS.customise = {
  render(app) {
    app.innerHTML =
      '<div class="page"><div class="page-inner fade-in">' +
      '<div class="page-head"><div><h1>Customise</h1><p>Encoder, audio and scene settings.</p></div>' +
      '<button class="btn" id="cu-studio">' +
      icon("video") +
      "Open Studio</button></div>" +
      '<div class="cu-banner" id="cu-banner" hidden></div>' +
      '<div class="cu-layout"><nav class="cu-nav" id="cu-nav"></nav><div class="card cu-pane" id="cu-pane"></div></div>' +
      "</div></div>";
    $("#cu-studio").onclick = () => (S.accounts.length ? openStudio() : go("login"));
    drawCustomise();
    loadSceneLibrary().then(() => {
      if (S.view === "customise" && Cu.tab === "scenes") drawCustomise();
    });
  },
  leave() {
    clearInterval(Cu.timer);
  },
  tick() {
    drawCuBanner();
  },
  devices() {
    if (Cu.tab === "encoder" || Cu.tab === "audio" || Cu.tab === "system") drawCustomise();
  },
};

function drawCustomise() {
  clearInterval(Cu.timer);
  const nav = $("#cu-nav");
  if (!nav) return;
  nav.innerHTML = CU_TABS.map(
    (t) =>
      '<button data-t="' +
      t.id +
      '" class="' +
      (t.id === Cu.tab ? "on" : "") +
      '">' +
      icon(t.icon) +
      t.label +
      "</button>",
  ).join("");
  $$("button", nav).forEach(
    (b) =>
      (b.onclick = () => {
        Cu.tab = b.dataset.t;
        LS.set("custTab", Cu.tab);
        drawCustomise();
      }),
  );
  const pane = $("#cu-pane");
  (({ encoder: cuEncoder, audio: cuAudio, scenes: cuScenes, system: cuSystem })[Cu.tab] || cuEncoder)(pane);
  drawCuBanner();
}

function drawCuBanner() {
  const b = $("#cu-banner");
  if (!b) return;
  const live = liveCount() > 0;
  b.hidden = !live;
  if (live)
    b.innerHTML =
      icon("info") +
      "<span>You're LIVE. New settings apply the next time a LIVE starts, or when you press <b>Apply to LIVE</b> in Studio.</span>";
}

// Mark the LIVE so Studio offers "Apply to LIVE".
function encChanged() {
  saveEnc();
  St.pendingApply = St.pendingApply || {};
  for (const id of Object.keys(S.live)) St.pendingApply[id] = true;
}

function sliderRow(id, title, sub, min, max, step, value, fmtV) {
  return (
    '<div class="setting-row col-row"><div class="row" style="justify-content:space-between"><div class="sl"><b>' +
    title +
    "</b><span>" +
    sub +
    "</span></div>" +
    '<span class="fixed-pill" id="' +
    id +
    '-v">' +
    fmtV(value) +
    "</span></div>" +
    '<input type="range" class="range" id="' +
    id +
    '" min="' +
    min +
    '" max="' +
    max +
    '" step="' +
    step +
    '" value="' +
    value +
    '"></div>'
  );
}
function wireSlider(id, fmtV, onChange) {
  const r = $("#" + id);
  paintRange(r);
  r.oninput = () => {
    paintRange(r);
    $("#" + id + "-v").textContent = fmtV(+r.value);
    onChange(+r.value, false);
  };
  r.onchange = () => onChange(+r.value, true);
}

const ENCODER_VENDOR = { h264_nvenc: "nvidia", h264_amf: "amd", h264_qsv: "intel", h264_videotoolbox: "apple" };

function encoderHint() {
  if (!S.devices.loaded) return "Checking your graphics card…";
  if (!S.devices.ffmpeg) return "ffmpeg isn't installed yet.";
  const hw = S.devices.hardware || {};
  const gpus = hw.gpus || [];
  const def = S.devices.encoders.default;
  if (def && def !== "libx264") {
    const gpu = gpus.find((g) => g.vendor === ENCODER_VENDOR[def]) || gpus[0];
    return "Picked for your " + esc(gpu ? gpu.name : "graphics card") + ", so your CPU stays free while you stream.";
  }
  return (
    (gpus.length ? esc(gpus[0].name) + " can't encode video here" : "No graphics card encoder found") +
    ", so x264 runs on your " +
    esc(hw.cpu || "CPU") +
    ". Lower the bitrate if your stream stutters."
  );
}

function gpuLabel(g) {
  return g.name + (g.vram_mb ? " · " + Math.round(g.vram_mb / 1024) + " GB" : "");
}

function cuEncoder(pane) {
  const av = S.devices.encoders.available || ["libx264"];
  const cur = activeCodec();
  const kbps = (v) => v + " kbps";
  pane.innerHTML =
    '<h2 class="pane-title">Encoder</h2>' +
    '<div class="setting-row"><div class="sl"><b>Video encoder</b><span>' +
    encoderHint() +
    '</span></div><select class="select" id="cu-codec" style="width:260px">' +
    av
      .map(
        (c) =>
          '<option value="' +
          c +
          '"' +
          (c === cur ? " selected" : "") +
          ">" +
          esc(codecLabel(c)) +
          (c === S.devices.encoders.default ? " · recommended" : "") +
          "</option>",
      )
      .join("") +
    "</select></div>" +
    sliderRow(
      "cu-br",
      "Bitrate",
      "Higher looks sharper but needs more upload speed.",
      2500,
      12000,
      500,
      E.bitrate,
      kbps,
    ) +
    '<div class="chips" style="margin:-4px 0 6px"><button class="chip" data-br="4000">4000 · Saver</button><button class="chip" data-br="6000">6000 · Recommended</button><button class="chip" data-br="8000">8000 · High</button><button class="chip" data-br="10000">10000 · Max</button></div>' +
    '<div class="setting-row"><div class="sl"><b>Frame rate</b><span>Every LIVE runs at 60 FPS, like TikTok LIVE Studio.</span></div><span class="fixed-pill">60 FPS</span></div>' +
    '<div class="setting-row"><div class="sl"><b>Resolution</b><span>Set by the scene layout in Studio.</span></div><span class="fixed-pill">1080×1920 · 1920×1080</span></div>';
  $("#cu-codec").onchange = (e) => {
    E.codec = e.target.value;
    encChanged();
    toast("Encoder set to " + codecLabel(E.codec), "ok");
  };
  const drawChips = () => $$("[data-br]", pane).forEach((c) => c.classList.toggle("on", +c.dataset.br === E.bitrate));
  wireSlider("cu-br", kbps, (v, done) => {
    E.bitrate = v;
    drawChips();
    if (done) encChanged();
  });
  $$("[data-br]", pane).forEach(
    (c) =>
      (c.onclick = () => {
        E.bitrate = +c.dataset.br;
        const r = $("#cu-br");
        r.value = E.bitrate;
        paintRange(r);
        $("#cu-br-v").textContent = kbps(E.bitrate);
        drawChips();
        encChanged();
      }),
  );
  drawChips();
}

function cuAudio(pane) {
  const mics = S.devices.mics || [];
  const pct = (v) => Math.round(v * 100) + "%";
  pane.innerHTML =
    '<h2 class="pane-title">Audio</h2>' +
    '<div class="setting-row"><div class="sl"><b>Microphone</b><span>' +
    (mics.length
      ? "Mixed into every LIVE you start."
      : S.devices.loaded
        ? "No microphones found."
        : "Looking for microphones…") +
    "</span></div>" +
    '<select class="select" id="cu-mic" style="width:300px"><option value="">Off</option>' +
    mics
      .map((m) => '<option value="' + esc(m) + '"' + (m === E.mic ? " selected" : "") + ">" + esc(m) + "</option>")
      .join("") +
    (E.mic && !mics.includes(E.mic)
      ? '<option value="' + esc(E.mic) + '" selected>' + esc(E.mic) + " (not connected)</option>"
      : "") +
    "</select></div>" +
    sliderRow("cu-micvol", "Microphone volume", "100% = unchanged.", 0, 2, 0.05, E.micVol, pct) +
    sliderRow(
      "cu-vidvol",
      "Video audio volume",
      "Sound from video sources that have “Use video audio” on.",
      0,
      2,
      0.05,
      E.vidVol,
      pct,
    ) +
    '<p class="hint" style="margin-top:14px">Desktop audio capture isn\'t available yet. To stream game or system sound, use a video source with audio, or route it to a virtual microphone.</p>';
  $("#cu-mic").onchange = (e) => {
    E.mic = e.target.value;
    encChanged();
  };
  wireSlider("cu-micvol", pct, (v, done) => {
    E.micVol = v;
    if (done) encChanged();
  });
  wireSlider("cu-vidvol", pct, (v, done) => {
    E.vidVol = v;
    if (done) encChanged();
  });
}

function cuScenes(pane) {
  const list = S.scenes || [];
  pane.innerHTML =
    '<div class="row" style="justify-content:space-between;margin-bottom:6px"><h2 class="pane-title" style="margin:0">Saved scenes</h2>' +
    '<span class="hint">Save a scene from Studio with the ⋯ menu next to the scene name.</span></div>' +
    (list.length
      ? '<div class="scene-list">' +
        list
          .map((sc, i) => {
            const c = sc.config || {};
            const land = (c.width || 1080) > (c.height || 1920);
            const srcs = c.sources || [];
            const kinds = [...new Set(srcs.map((s) => s.kind))];
            return (
              '<div class="scene-card"><div class="sc-thumb' +
              (land ? " land" : "") +
              '">' +
              icon(kinds.length === 1 && KIND[kinds[0]] ? KIND[kinds[0]].icon : "layers") +
              "</div>" +
              '<div class="grow"><b>' +
              esc(sc.name) +
              "</b><span>" +
              srcs.length +
              " source" +
              (srcs.length === 1 ? "" : "s") +
              " · " +
              (land ? "Landscape" : "Mobile") +
              (kinds.length ? " · " + kinds.map((k) => (KIND[k] ? KIND[k].label : k)).join(", ") : "") +
              "</span></div>" +
              '<button class="btn sm" data-load="' +
              i +
              '">Use in Studio</button><button class="icon-btn sm" data-del="' +
              i +
              '" title="Delete">' +
              icon("trash") +
              "</button></div>"
            );
          })
          .join("") +
        "</div>"
      : '<div class="empty big-empty">' +
        icon("layers", "ico big") +
        "<b>No saved scenes</b><p>Build a scene in Studio, then save it to reuse it later.</p></div>");
  $$("[data-load]", pane).forEach(
    (b) =>
      (b.onclick = () => {
        const sc = list[+b.dataset.load];
        if (!S.accounts.length) return toast("Log in first", "err");
        openStudio();
        setTimeout(() => loadSavedScene(sc), 30);
      }),
  );
  $$("[data-del]", pane).forEach(
    (b) =>
      (b.onclick = async () => {
        const sc = list[+b.dataset.del];
        if (
          !(await confirmBox(
            'Delete "' + esc(sc.name) + '"?',
            "The saved scene is removed. Scenes already open in Studio aren't affected.",
            "Delete",
          ))
        )
          return;
        try {
          S.scenes = (await api("DELETE", "/api/scenes/" + encodeURIComponent(sc.name))).scenes || [];
          toast("Scene deleted", "ok");
          drawCustomise();
        } catch (e) {
          toast(e.message, "err");
        }
      }),
  );
}

function cuSystem(pane) {
  const ff = S.devices.ffmpeg;
  const hw = S.devices.hardware || {};
  pane.innerHTML =
    '<h2 class="pane-title">System</h2>' +
    '<div class="stat-tiles" style="margin-bottom:18px"><div class="stat-tile"><b id="cu-cpu">—</b><span>CPU</span></div><div class="stat-tile"><b id="cu-mem">—</b><span>Memory</span></div><div class="stat-tile"><b>' +
    (liveCount() ? "LIVE" : "Offline") +
    "</b><span>Status</span></div></div>" +
    '<div class="kv"><span>Processor</span><b>' +
    (hw.cpu ? esc(hw.cpu) + " · " + hw.threads + " threads" : "Checking…") +
    "</b></div>" +
    (hw.gpus && hw.gpus.length ? hw.gpus : [{ name: S.devices.loaded ? "None found" : "Checking…" }])
      .map((g, i) => '<div class="kv"><span>' + (i ? "" : "Graphics") + "</span><b>" + esc(gpuLabel(g)) + "</b></div>")
      .join("") +
    '<div class="kv"><span>ffmpeg</span>' +
    (!S.devices.loaded
      ? "<b>Checking…</b>"
      : ff
        ? '<b title="' + esc(ff) + '">' + esc(ff) + "</b>"
        : '<button class="btn sm primary" id="cu-ffmpeg">Install ffmpeg</button>') +
    "</div>" +
    '<div class="kv"><span>GPU encoders</span><b>' +
    esc(
      (S.devices.encoders.available || [])
        .filter((c) => c !== "libx264")
        .map(codecLabel)
        .join(", ") || "None",
    ) +
    "</b></div>" +
    '<div class="kv"><span>Microphones</span><b>' +
    (S.devices.mics || []).length +
    "</b></div>" +
    '<div class="kv"><span>Account</span><b>' +
    (acct() ? esc(nameOf(acct())) : "Not logged in") +
    "</b></div>" +
    '<div class="kv"><span>Edition</span><b>Free</b></div>' +
    '<div class="kv"><span>Version</span><b>1.0.0</b></div>' +
    '<p class="hint" style="margin-top:14px">TikTok requests use your own internet connection. The video goes straight to TikTok\'s ingest server, the same as TikTok LIVE Studio.</p>';
  if ($("#cu-ffmpeg")) $("#cu-ffmpeg").onclick = openFFmpegSetup;
  const tick = async () => {
    try {
      const s = await api("GET", "/api/system");
      if ($("#cu-cpu")) {
        $("#cu-cpu").textContent = (s.cpu || 0).toFixed(0) + "%";
        $("#cu-mem").textContent = (s.mem || 0).toFixed(0) + "%";
      }
    } catch {}
  };
  tick();
  Cu.timer = setInterval(tick, 2000);
}
