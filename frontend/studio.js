// Studio tab: scene, canvas, dock, stats and chat.
"use strict";

const St = { sel: null, dirtyLive: false, frameSeq: 0, frameURL: null, loop: 0, timers: [] };

// The working scene is stored locally.
function blankScene() {
  return { name: "My scene", orientation: "portrait", sources: [] };
}
function scene() {
  if (!St.scene || St.sceneAcct !== S.acctId) {
    St.scene = LS.get("scene." + S.acctId, null) || blankScene();
    St.sceneAcct = S.acctId;
  }
  return St.scene;
}
function saveScene() {
  LS.set("scene." + S.acctId, scene());
}
function canvasSize() {
  return scene().orientation === "landscape" ? [1920, 1080] : [1080, 1920];
}
function visibleSources() {
  return scene().sources.filter((s) => !s.hidden);
}
function sceneConfig() {
  const [W, H] = canvasSize();
  return {
    width: W,
    height: H,
    fps: 60,
    bitrate: E.bitrate,
    codec: activeCodec(),
    audio_device: E.mic || "",
    video_volume: +E.vidVol,
    mic_volume: +E.micVol,
    sources: scene().sources.map((s) => ({
      kind: s.kind,
      name: s.name || "",
      hidden: !!s.hidden,
      path: s.path || "",
      color: s.color || "",
      text: s.text || "",
      font_size: s.font_size | 0,
      x: s.x | 0,
      y: s.y | 0,
      w: s.w | 0,
      h: s.h | 0,
      fit: s.fit || "",
      use_audio: !!s.use_audio,
      monitor: s.monitor | 0,
      cursor: !!s.cursor,
      window: s.window || "",
      window_app: s.window_app || "",
      window_title: s.window_title || "",
      crop_l: s.crop_l | 0,
      crop_t: s.crop_t | 0,
      crop_r: s.crop_r | 0,
      crop_b: s.crop_b | 0,
      flip_h: !!s.flip_h,
      flip_v: !!s.flip_v,
      volume: s.volume == null ? undefined : +s.volume,
      muted: !!s.muted,
    })),
  };
}
const KIND = {
  video: { icon: "film", label: "Video" },
  image: { icon: "image", label: "Image" },
  screen: { icon: "monitor", label: "Screen / window" },
  color: { icon: "color", label: "Color" },
  text: { icon: "text", label: "Text" },
};
function sourceTitle(s) {
  if (s.name) return s.name;
  if (s.kind === "text") return s.text ? '"' + s.text.slice(0, 24) + '"' : "Text";
  if (s.kind === "color") return "Color " + (s.color || "white");
  if (s.kind === "screen")
    return s.window ? s.window_title || s.window_app || "Window" : "Screen " + ((s.monitor | 0) + 1);
  if (s.path) return s.path.split(/[\\/]/).pop();
  return KIND[s.kind].label;
}

function openStudio(id) {
  if (id) {
    S.acctId = id;
    LS.set("lastAcct", id);
  }
  if (!acct()) {
    if (S.accounts.length) S.acctId = S.accounts[0].id;
    else return go("login");
  }
  if (S.view === "studio") {
    VIEWS.studio.leave();
    go("studio");
  } else go("studio");
}

VIEWS.studio = {
  render(app) {
    St.sel = null;
    St.dirtyLive = !!(St.pendingApply && St.pendingApply[S.acctId]);
    if (St.pendingApply) delete St.pendingApply[S.acctId];
    const a = acct();
    app.innerHTML =
      '<div class="studio">' +
      '<div class="col">' +
      '<section class="panel scene-panel">' +
      '<button class="acct-switch" id="st-acct"><img src="' +
      esc(avatarOf(a)) +
      '" onerror="this.style.visibility=\'hidden\'"><div class="grow"><b>' +
      esc(nameOf(a)) +
      "</b><span>@" +
      esc(handleOf(a)) +
      "</span></div>" +
      icon("down") +
      "</button>" +
      '<div class="seg" id="st-orient" style="margin-top:12px"><button data-o="portrait">' +
      icon("phone") +
      'Mobile view</button><button data-o="landscape">' +
      icon("land") +
      "Landscape</button></div>" +
      '<div class="scene-head"><button class="scene-name" id="st-scene"><span></span>' +
      icon("down") +
      '</button><button class="icon-btn sm" id="st-scene-more">' +
      icon("more") +
      "</button></div>" +
      '<ul class="src-list" id="st-srcs"></ul>' +
      '<button class="add-source" id="st-add">' +
      icon("plus") +
      "Add source</button>" +
      "</section>" +
      '<section class="panel tools-panel" id="st-tools"></section>' +
      "</div>" +
      '<div class="center">' +
      '<div class="canvas-area" id="st-area">' +
      '<div class="live-chip" id="st-livechip" hidden><span class="pulse"></span><span>LIVE</span><span id="st-timer">0:00</span></div>' +
      '<div class="stage" id="st-stage"><img class="frame" id="pv-frame" hidden><div class="layers" id="st-layers"></div><div class="ph" id="pv-ph"></div></div>' +
      '<div class="canvas-tools"><button class="icon-btn" id="st-settings" title="Encoder settings">' +
      icon("settings") +
      "</button></div>" +
      '<div class="xf-bar" id="xf-bar" hidden></div>' +
      '<div class="apply-banner" id="st-apply" hidden><span>Scene changed</span><button class="btn sm primary" id="st-apply-btn">Apply to LIVE</button></div>' +
      "</div>" +
      '<div class="dock">' +
      '<button class="icon-btn" id="dk-add" title="Add source">' +
      icon("plus") +
      "</button>" +
      '<button class="icon-btn" id="dk-title" title="LIVE title">' +
      icon("edit") +
      "</button>" +
      '<button class="icon-btn" id="dk-share" title="Copy LIVE link">' +
      icon("share") +
      "</button>" +
      '<span class="div"></span>' +
      '<div class="vol" title="Video audio volume">' +
      icon("volume") +
      '<input type="range" class="range" id="dk-vid" min="0" max="2" step="0.05"></div>' +
      '<div class="vol" title="Microphone volume">' +
      icon("mic") +
      '<input type="range" class="range" id="dk-mic" min="0" max="2" step="0.05"></div>' +
      '<button class="go-live" id="dk-live">Go LIVE</button>' +
      "</div>" +
      '<div class="statusline" id="st-status"></div>' +
      "</div>" +
      '<div class="col">' +
      '<section class="panel perf"><div class="panel-title">LIVE performance <span class="hint" id="pf-room"></span></div><div class="perf-grid" id="pf-grid"></div></section>' +
      '<section class="panel rank-panel"><div class="panel-title"><div class="ptabs" id="rp-tabs"><button data-p="chat">Chat</button><button data-p="rank">Top viewers</button></div>' +
      '<button class="icon-btn sm" id="ch-filter" title="Choose what shows in chat">' +
      icon("more") +
      "</button></div>" +
      '<div class="chat-wrap" id="ch-wrap"><div class="ch-err" id="ch-err" hidden></div><div class="chat-list" id="ch-list"></div><button class="ch-new" id="ch-new" hidden>New messages ' +
      icon("down", "ico xs") +
      "</button></div>" +
      '<div class="rank-list" id="pf-rank" hidden></div></section>' +
      "</div></div>";

    $("#st-acct").onclick = (e) => accountMenu(e.currentTarget);
    $$("#st-orient button").forEach((b) => (b.onclick = () => setOrientation(b.dataset.o)));
    $("#st-scene").onclick = (e) => sceneMenu(e.currentTarget);
    $("#st-scene-more").onclick = (e) => sceneMenu(e.currentTarget);
    $("#st-add").onclick = openAddSource;
    $("#dk-add").onclick = openAddSource;
    $("#dk-title").onclick = editTitle;
    $("#dk-share").onclick = shareLive;
    $("#st-settings").onclick = () => go("customise");
    $("#dk-live").onclick = () => (isLive() ? endLiveFlow(S.acctId) : startGoLive());
    $("#st-apply-btn").onclick = (e) => applySceneToLive(e.currentTarget);
    $$("#rp-tabs button").forEach((b) => (b.onclick = () => setRightTab(b.dataset.p)));
    wireChatPanel();
    setRightTab(LS.get("rpTab", "chat"));
    chatStart();
    wireVolume("dk-vid", "vidVol");
    wireVolume("dk-mic", "micVol");
    wireStage();
    document.addEventListener("keydown", studioKeys);
    renderTools();
    drawStudio();
    St.ro = new ResizeObserver(layoutStage);
    St.ro.observe($("#st-area"));
    layoutStage();
    startFrameLoop();
    refreshPreview(true);
    layersStart();
    syncHear();
    loadSceneLibrary();
    St.timers.push(setInterval(statusTick, 1500), setInterval(perfTick, 5000), setInterval(liveClock, 1000));
    statusTick();
    perfTick();
  },
  leave() {
    St.timers.forEach(clearInterval);
    chatStop();
    layersStop();
    stopHear();
    document.removeEventListener("keydown", studioKeys);
    XF.crop = false;
    St.timers = [];
    St.loop++;
    if (St.ro) St.ro.disconnect();
    if (S.acctId && !isLive())
      api("POST", "/api/accounts/" + encodeURIComponent(S.acctId) + "/preview/stop").catch(() => {});
  },
  tick() {
    drawLiveBits();
  },
};

function drawStudio() {
  const sc = scene();
  $$("#st-orient button").forEach((b) => b.classList.toggle("on", b.dataset.o === sc.orientation));
  $("#st-scene span").textContent = sc.name || "My scene";
  drawSources();
  drawLiveBits();
  drawPerf();
  const vid = $("#dk-vid"),
    mic = $("#dk-mic");
  vid.value = E.vidVol;
  mic.value = E.micVol;
  paintRange(vid);
  paintRange(mic);
}

function drawLiveBits() {
  if (S.view !== "studio") return;
  const live = isLive();
  const b = $("#dk-live");
  b.textContent = live ? "End LIVE" : "Go LIVE";
  b.classList.toggle("end", live);
  $("#st-livechip").hidden = !live;
  if (St.wasLive !== live) {
    St.wasLive = live;
    drawPlaceholder();
    drawLayers();
    syncLayersSoon();
  }
  $("#st-apply").hidden = !(live && St.dirtyLive);
  $("#pf-room").textContent = live ? "" : "Offline";
  const a = acct();
  const img = $("#st-acct img");
  if (img && a) img.src = avatarOf(a);
}

function liveClock() {
  const s = S.live[S.acctId];
  if (s && $("#st-timer")) $("#st-timer").textContent = hms(Date.now() / 1000 - (s.started_at || Date.now() / 1000));
}

function accountMenu(anchor) {
  const a = acct();
  openMenu(anchor, [
    { header: nameOf(a) },
    { icon: "grid", label: "Dashboard", onClick: () => go("dashboard") },
    { icon: "refresh", label: "Refresh profile", onClick: () => refreshProfile(a) },
  ]);
}

// Saved scenes
async function loadSceneLibrary() {
  try {
    S.scenes = (await api("GET", "/api/scenes")).scenes || [];
  } catch {
    S.scenes = [];
  }
}
function sceneMenu(anchor) {
  openMenu(anchor, [
    { header: "Saved scenes" },
    ...(S.scenes.length
      ? S.scenes.map((sc) => ({ icon: "layers", label: sc.name, onClick: () => loadSavedScene(sc) }))
      : [{ label: "No saved scenes", disabled: true }]),
    "-",
    { icon: "save", label: "Save scene as…", onClick: saveSceneAs },
    { icon: "edit", label: "Rename scene", onClick: renameScene },
    { icon: "trash", label: "Clear scene", danger: true, onClick: clearScene },
  ]);
}
function loadSavedScene(sc) {
  const c = sc.config || {};
  const land = (c.width || 1080) > (c.height || 1920);
  St.scene = {
    name: sc.name,
    orientation: land ? "landscape" : "portrait",
    sources: (c.sources || []).map((s) => Object.assign({ id: uid() }, s)),
  };
  if (c.bitrate) E.bitrate = c.bitrate;
  saveEnc();
  saveScene();
  St.sel = null;
  drawStudio();
  sceneChanged();
  toast('Loaded "' + sc.name + '"', "ok");
}
function saveSceneAs() {
  const m = openModal({
    title: "Save scene",
    body:
      '<div class="field"><label>Scene name</label><input class="input" id="sv-name" value="' +
      esc(scene().name) +
      '"></div>',
    foot: [
      {
        label: "Save",
        cls: "primary",
        fn: async (btn) => {
          const name = $("#sv-name").value.trim();
          if (!name) return toast("Enter a name", "err");
          await withSpinner(btn, "", async () => {
            const cfg = sceneConfig();
            S.scenes = (await api("POST", "/api/scenes", { name, config: cfg })).scenes || [];
            scene().name = name;
            saveScene();
            drawStudio();
            m.close();
            toast("Scene saved", "ok");
          });
        },
      },
    ],
  });
  setTimeout(() => $("#sv-name") && $("#sv-name").select(), 30);
}
function renameScene() {
  const m = openModal({
    title: "Rename scene",
    body:
      '<div class="field"><label>Name</label><input class="input" id="rn-name" value="' +
      esc(scene().name) +
      '"></div>',
    foot: [
      {
        label: "Rename",
        cls: "primary",
        fn: () => {
          const v = $("#rn-name").value.trim();
          if (v) {
            scene().name = v;
            saveScene();
            drawStudio();
          }
          m.close();
        },
      },
    ],
  });
}
async function clearScene() {
  if (!(await confirmBox("Clear scene?", "Remove every source from this scene."))) return;
  scene().sources = [];
  St.sel = null;
  saveScene();
  drawStudio();
  sceneChanged();
}

function setOrientation(o) {
  if (scene().orientation === o) return;
  scene().orientation = o;
  saveScene();
  drawStudio();
  layoutStage();
  sceneChanged();
}

function drawSources() {
  const ul = $("#st-srcs");
  const list = scene().sources;
  if (!list.length) {
    ul.innerHTML = '<li class="empty" style="padding:14px 6px">Add a video, image, screen, color or text.</li>';
    return;
  }
  // top of the list is the front layer, as in TikTok LIVE Studio
  ul.innerHTML = list
    .slice()
    .reverse()
    .map(
      (s) =>
        '<li class="src' +
        (St.sel === s.id ? " sel" : "") +
        (s.hidden ? " hidden-src" : "") +
        '" data-id="' +
        s.id +
        '">' +
        icon(KIND[s.kind].icon, "ico si") +
        '<span class="sn">' +
        esc(sourceTitle(s)) +
        "</span>" +
        (hasSound(s)
          ? '<button class="aud-btn' +
            (s.muted ? " muted" : "") +
            '" data-a="aud" title="Video sound: ' +
            (s.muted ? "muted" : pct(srcVolume(s))) +
            '">' +
            icon(audioIcon(s), "ico xs") +
            "<span>" +
            (s.muted ? "Muted" : pct(srcVolume(s))) +
            "</span></button>"
          : "") +
        '<span class="sa"><button class="icon-btn sm" data-a="vis" title="' +
        (s.hidden ? "Show" : "Hide") +
        '">' +
        icon(s.hidden ? "eyeoff" : "eye") +
        "</button>" +
        '<button class="icon-btn sm" data-a="more" title="More">' +
        icon("more") +
        "</button></span></li>",
    )
    .join("");
  $$(".src", ul).forEach((li) => {
    const s = list.find((x) => x.id === li.dataset.id);
    li.onclick = (e) => {
      const a = e.target.closest("[data-a]");
      if (a && a.dataset.a === "vis") {
        s.hidden = !s.hidden;
        saveScene();
        drawSources();
        renderSelBox();
        sceneChanged();
        return;
      }
      if (a && a.dataset.a === "more") return sourceMenu(a, s);
      if (a && a.dataset.a === "aud") return openAudioPop(a, s);
      selectSource(s.id);
    };
    li.ondblclick = () => editSource(s);
  });
}
function selectSource(id) {
  St.sel = id;
  drawSources();
  renderSelBox();
}
function sourceMenu(anchor, s) {
  const list = scene().sources,
    i = list.indexOf(s);
  openMenu(
    anchor,
    [
      { icon: "edit", label: "Edit", onClick: () => editSource(s) },
      { icon: "up", label: "Bring forward", disabled: i === list.length - 1, onClick: () => moveSource(s, 1) },
      { icon: "down", label: "Send backward", disabled: i === 0, onClick: () => moveSource(s, -1) },
      {
        icon: "copy",
        label: "Duplicate",
        onClick: () => {
          list.splice(i + 1, 0, Object.assign({}, s, { id: uid() }));
          saveScene();
          drawSources();
          sceneChanged();
        },
      },
      "-",
      {
        icon: "trash",
        label: "Remove",
        danger: true,
        onClick: () => {
          list.splice(i, 1);
          if (St.sel === s.id) St.sel = null;
          saveScene();
          drawSources();
          renderSelBox();
          sceneChanged();
        },
      },
    ],
    "right",
  );
}
function moveSource(s, dir) {
  const list = scene().sources,
    i = list.indexOf(s),
    j = i + dir;
  if (j < 0 || j >= list.length) return;
  list.splice(i, 1);
  list.splice(j, 0, s);
  saveScene();
  drawSources();
  sceneChanged();
}

function sceneChanged() {
  if (isLive()) {
    St.dirtyLive = true;
    drawLiveBits();
    return;
  }
  drawPlaceholder();
  drawLayers();
  syncLayersSoon();
}
// When not LIVE the canvas is the layer editor (layers.js). While LIVE it shows
// the stream's own preview output.
async function refreshPreview() {
  const id = S.acctId;
  drawPlaceholder();
  if (isLive(id)) return;
  api("POST", "/api/accounts/" + encodeURIComponent(id) + "/preview/stop").catch(() => {});
  drawLayers();
  syncLayersSoon();
}
async function applySceneToLive(btn) {
  await withSpinner(btn, "", async () => {
    const id = S.acctId;
    try {
      await api("POST", "/api/accounts/" + encodeURIComponent(id) + "/encoder/stop").catch(() => {});
      await api("POST", "/api/accounts/" + encodeURIComponent(id) + "/encoder/start", { config: sceneConfig() });
      St.dirtyLive = false;
      drawLiveBits();
      toast("Scene applied to your LIVE", "ok");
    } catch (e) {
      toast(e.message, "err");
    }
  });
}

function layoutStage() {
  const area = $("#st-area"),
    stage = $("#st-stage");
  if (!area || !stage) return;
  const [W, H] = canvasSize();
  const aw = area.clientWidth - 40,
    ah = area.clientHeight - 28;
  const scale = Math.max(0.05, Math.min(aw / W, ah / H));
  stage.style.width = Math.round(W * scale) + "px";
  stage.style.height = Math.round(H * scale) + "px";
  stage.classList.toggle("land", W > H);
  St.scale = scale;
  drawLayers();
  renderSelBox();
}
function drawPlaceholder() {
  const ph = $("#pv-ph"),
    img = $("#pv-frame");
  if (!ph) return;
  const empty = !visibleSources().length;
  if (empty) {
    img.hidden = true;
    ph.hidden = false;
  } else if (!isLive())
    ph.hidden = true; // the layers are the preview
  else if (img.hidden) ph.hidden = false;
  ph.innerHTML = empty
    ? '<div class="big">' +
      icon("layers") +
      "</div><b style='color:var(--text-2)'>Your scene is empty</b><span>Add a video, image or screen to start.</span>" +
      '<button class="btn sm primary" onclick="openAddSource()">' +
      icon("plus") +
      "Add source</button>"
    : '<span class="spin"></span><span>Starting preview…</span>';
}
async function startFrameLoop() {
  const token = ++St.loop;
  St.frameSeq = 0;
  while (token === St.loop) {
    const id = S.acctId;
    let got = false;
    try {
      const r = await fetch("/api/accounts/" + encodeURIComponent(id) + "/preview/frame?since=" + St.frameSeq, {
        cache: "no-store",
      });
      if (r.status === 200 && token === St.loop && id === S.acctId) {
        St.frameSeq = +r.headers.get("x-frame-seq") || St.frameSeq + 1;
        const url = URL.createObjectURL(await r.blob());
        const img = $("#pv-frame");
        if (img && isLive(id)) {
          img.src = url;
          img.hidden = false;
          $("#pv-ph").hidden = true;
        }
        if (St.frameURL) URL.revokeObjectURL(St.frameURL);
        St.frameURL = url;
        got = true;
      }
    } catch {}
    await sleep(got ? 35 : document.hidden ? 600 : 90);
  }
}

function paintRange(r) {
  r.style.setProperty("--p", ((r.value - r.min) / (r.max - r.min)) * 100 + "%");
}
function wireVolume(id, key) {
  const r = $("#" + id);
  r.oninput = () => {
    E[key] = +r.value;
    paintRange(r);
    r.title = Math.round(r.value * 100) + "%";
  };
  r.onchange = () => {
    saveEnc();
    syncHear();
    applyAudioLive();
  };
}

async function statusTick() {
  if (S.view !== "studio") return;
  let sys = { cpu: 0, mem: 0 },
    enc = null;
  try {
    sys = await api("GET", "/api/system");
  } catch {}
  if (isLive()) {
    try {
      enc = await api("GET", "/api/accounts/" + encodeURIComponent(S.acctId) + "/encoder/status");
    } catch {}
  }
  const el = $("#st-status");
  if (!el) return;
  const run = enc && enc.running;
  const drops = run ? enc.drops + "(" + (enc.drop_pct || 0).toFixed(1) + "%)" : "0(0.0%)";
  const fps = run ? Math.round(enc.fps || 0) : 0;
  const slow = run && enc.speed && enc.speed < 0.95;
  el.innerHTML =
    "<span>CPU: <b>" +
    (sys.cpu || 0).toFixed(1) +
    "%</b></span>" +
    "<span>Memory: <b>" +
    (sys.mem || 0).toFixed(0) +
    "%</b></span>" +
    "<span>Upload: <b>" +
    (run ? enc.upload_kbps : 0) +
    " kbps</b></span>" +
    "<span>Frame drops: <b>" +
    drops +
    "</b></span>" +
    '<span class="' +
    (slow ? "bad" : "") +
    '">FPS: <b>' +
    fps +
    "/60</b></span>" +
    (isLive() && enc && !enc.running ? '<span class="bad"><b>Reconnecting…</b></span>' : "");
}

function drawPerf() {
  const p = S.perf[S.acctId] || {};
  const r = St.rewards || {};
  const cell = (ic, v, label, cls = "") =>
    '<div class="perf-item ' + cls + '">' + icon(ic) + "<div>" + v + "<small>" + label + "</small></div></div>";
  const grid = $("#pf-grid");
  if (!grid) return;
  grid.innerHTML =
    cell("diamond", fmt(r.diamonds || p.diamonds), "Diamonds", "dia") +
    cell("dollar", r.amount ? r.amount : "0.00", "Rewards") +
    cell("users", viewersNow(p) == null ? "—" : fmt(viewersNow(p)), "Viewers now") +
    cell("userplus", fmt(p.new_fans), "New followers") +
    cell("heart", fmt(p.likes), "Likes") +
    cell("gift", fmt(p.gifters), "Gifters");
}
async function perfTick() {
  if (S.view !== "studio") return;
  const id = S.acctId;
  if (!isLive(id)) {
    St.rewards = null;
    drawPerf();
    drawRank([]);
    return;
  }
  const enc = encodeURIComponent(id);
  const [stats, rewards, aud] = await Promise.all([
    api("GET", "/api/accounts/" + enc + "/stats").catch(() => null),
    api("GET", "/api/accounts/" + enc + "/rewards").catch(() => null),
    api("GET", "/api/accounts/" + enc + "/audience").catch(() => null),
  ]);
  if (id !== S.acctId) return;
  if (stats) S.perf[id] = Object.assign(S.perf[id] || {}, stats);
  St.rewards = rewards;
  drawPerf();
  drawRank((aud && aud.viewers) || []);
}
function setRightTab(p) {
  LS.set("rpTab", p);
  $$("#rp-tabs button").forEach((b) => b.classList.toggle("on", b.dataset.p === p));
  $("#ch-wrap").hidden = p !== "chat";
  $("#ch-filter").hidden = p !== "chat";
  $("#pf-rank").hidden = p !== "rank";
  if (p === "chat") drawChat();
}
function drawRank(list) {
  const el = $("#pf-rank");
  if (!el) return;
  if (!isLive()) {
    el.innerHTML = '<div class="empty">Go LIVE to see your top viewers.</div>';
    return;
  }
  if (!list.length) {
    el.innerHTML = '<div class="empty">No ranked viewers yet.</div>';
    return;
  }
  el.innerHTML = list
    .slice(0, 30)
    .map(
      (v) =>
        '<div class="rank"><span class="no ' +
        (v.rank <= 3 ? "top" : "") +
        '">' +
        v.rank +
        "</span>" +
        '<img src="' +
        esc(cdn(v.avatar)) +
        '" onerror="this.style.visibility=\'hidden\'"><span class="nm">' +
        esc(v.name) +
        '</span><span class="sc">' +
        fmt(v.score) +
        "</span></div>",
    )
    .join("");
}
