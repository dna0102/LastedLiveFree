// Scene sources: adding, editing, and moving/resizing/cropping on the canvas.
"use strict";

// Image, video, screen and color sources are drawn exactly into their box
// {x, y, w, h} (canvas pixels) after cropping (source pixels); fit is always
// "stretch". Fit/Fill/Stretch in the UI just set the box. Older scenes used
// fit "contain"/"cover"; normalize() converts those when first edited.
const XF_KINDS = ["image", "video", "screen", "color"];
const canCrop = (s) => s.kind === "image" || s.kind === "video" || s.kind === "screen";

function cropped(s) {
  return [
    Math.max(2, (s.nat_w || 0) - (s.crop_l | 0) - (s.crop_r | 0)),
    Math.max(2, (s.nat_h || 0) - (s.crop_t | 0) - (s.crop_b | 0)),
  ];
}
function legacyFit(s) {
  return s.fit || (s.kind === "image" ? "cover" : "contain");
}

// Convert an old contain/cover layer to an exact box (plus crop, for cover).
function normalize(s) {
  if (!XF_KINDS.includes(s.kind) || legacyFit(s) === "stretch") return;
  const [W, H] = canvasSize();
  const b = { x: s.w ? s.x | 0 : 0, y: s.h ? s.y | 0 : 0, w: s.w || W, h: s.h || H };
  if (s.kind === "color" || !s.nat_w) {
    Object.assign(s, b, { fit: "stretch" });
    return;
  }
  const [nw, nh] = cropped(s);
  if (legacyFit(s) === "cover") {
    const k = Math.max(b.w / nw, b.h / nh),
      ex = (nw * k - b.w) / k,
      ey = (nh * k - b.h) / k;
    s.crop_l = (s.crop_l | 0) + Math.round(ex / 2);
    s.crop_r = (s.crop_r | 0) + Math.round(ex / 2);
    s.crop_t = (s.crop_t | 0) + Math.round(ey / 2);
    s.crop_b = (s.crop_b | 0) + Math.round(ey / 2);
    Object.assign(s, b);
  } else {
    const k = Math.min(b.w / nw, b.h / nh),
      w = Math.round(nw * k),
      h = Math.round(nh * k);
    Object.assign(s, { x: b.x + Math.round((b.w - w) / 2), y: b.y + Math.round((b.h - h) / 2), w, h });
  }
  s.fit = "stretch";
}

// fit: whole source visible; fill: covers the canvas; stretch: exactly the canvas
function placeFit(s, mode) {
  const [W, H] = canvasSize();
  if (mode === "stretch" || s.kind === "color" || !s.nat_w) Object.assign(s, { x: 0, y: 0, w: W, h: H });
  else {
    const [nw, nh] = cropped(s);
    const k = mode === "fill" ? Math.max(W / nw, H / nh) : Math.min(W / nw, H / nh);
    s.w = Math.round(nw * k);
    s.h = Math.round(nh * k);
    s.x = Math.round((W - s.w) / 2);
    s.y = Math.round((H - s.h) / 2);
  }
  s.fit = "stretch";
}
function centerSrc(s) {
  const [W, H] = canvasSize(),
    b = boxFor(s);
  if (s.kind === "text") {
    s.x = Math.round((W - b.w) / 2);
    s.y = Math.round((H - b.h) / 2);
    return;
  }
  s.x = Math.round((W - b.w) / 2);
  s.y = Math.round((H - b.h) / 2);
}

let CapCache = null;
async function captureSources(fresh) {
  if (!CapCache || fresh) CapCache = await api("GET", "/api/capture/sources");
  return CapCache;
}

// Look up the source's uncropped size (needed for aspect ratio and cropping).
async function ensureNat(s) {
  if (s.nat_w && s.nat_h) return true;
  try {
    if ((s.kind === "image" || s.kind === "video") && s.path) {
      const d = await api("GET", "/api/media/info?path=" + encodeURIComponent(s.path));
      s.nat_w = d.width;
      s.nat_h = d.height;
    } else if (s.kind === "screen") {
      const src = await captureSources();
      const t = s.window
        ? src.windows.find((x) => x.hwnd === s.window) ||
          src.windows.find((x) => x.app === s.window_app && x.title === s.window_title)
        : src.monitors.find((x) => x.index === (s.monitor | 0));
      if (t) {
        s.nat_w = t.width;
        s.nat_h = t.height;
      }
    }
  } catch {}
  return !!(s.nat_w && s.nat_h);
}

function openAddSource() {
  const m = openModal({
    title: "Add source",
    size: "wide",
    body:
      '<div class="src-kinds">' +
      Object.keys(KIND)
        .map((k) => '<button class="src-kind" data-k="' + k + '">' + icon(KIND[k].icon) + KIND[k].label + "</button>")
        .join("") +
      "</div>" +
      '<p class="hint" style="margin-top:14px">Video plays your clip on a loop until you end the LIVE. Screen capture can show a whole monitor or one app or browser window.</p>',
  });
  $$(".src-kind", m).forEach(
    (b) =>
      (b.onclick = () => {
        m.close();
        const k = b.dataset.k;
        if (k === "image" || k === "video") addMediaSource(k);
        else if (k === "screen") openCapturePicker(null, addCaptureSource);
        else editSource(null, k);
      }),
  );
}

async function addMediaSource(kind) {
  const path = await Native.pickFile(kind);
  if (!path) return;
  let info;
  try {
    info = await api("GET", "/api/media/info?path=" + encodeURIComponent(path));
  } catch (e) {
    return toast(e.message, "err");
  }
  const s = {
    id: uid(),
    kind,
    path,
    name: "",
    nat_w: info.width,
    nat_h: info.height,
    use_audio: kind === "video" && !!info.has_audio,
    fit: "stretch",
  };
  if (s.use_audio) {
    s.volume = DEFAULT_VIDEO_VOLUME;
    s.muted = false;
  } // unmuted, but quiet
  placeFit(s, "fit");
  addToScene(s);
}

function addCaptureSource(t) {
  const s = t.window
    ? {
        id: uid(),
        kind: "screen",
        window: t.window.hwnd,
        window_app: t.window.app,
        window_title: t.window.title,
        nat_w: t.window.width,
        nat_h: t.window.height,
      }
    : {
        id: uid(),
        kind: "screen",
        monitor: t.monitor.index,
        cursor: true,
        nat_w: t.monitor.width,
        nat_h: t.monitor.height,
      };
  s.fit = "stretch";
  placeFit(s, "fit");
  addToScene(s);
}

function addToScene(s) {
  scene().sources.push(s);
  St.sel = s.id;
  saveScene();
  drawSources();
  renderSelBox();
  sceneChanged();
  toast(sourceTitle(s) + " added — drag it to move, drag the corners to resize", "ok");
}

// Screen/window picker, with thumbnails.
async function openCapturePicker(current, onPick) {
  const m = openModal({
    title: "Screen capture",
    size: "xwide",
    sub: "Capture a whole screen, or just one app or browser window.",
    body:
      '<div class="row" style="margin-bottom:14px"><div class="seg" id="cp-tabs" style="width:280px"><button data-t="win">Windows</button><button data-t="mon">Screens</button></div>' +
      '<span class="grow"></span><button class="btn sm ghost" id="cp-refresh">' +
      icon("refresh") +
      "Refresh</button></div>" +
      '<div id="cp-grid" class="cap-grid"><div class="empty" style="grid-column:1/-1"><span class="spin" style="margin:auto"></span></div></div>',
  });
  let tab = current && current.monitor != null && !current.window ? "mon" : "win",
    data = null;
  const draw = () => {
    $$("#cp-tabs button").forEach((b) => b.classList.toggle("on", b.dataset.t === tab));
    const grid = $("#cp-grid");
    if (!grid || !data) return;
    if (tab === "mon") {
      grid.innerHTML = data.monitors.length
        ? data.monitors
            .map(
              (mo, i) =>
                '<button class="cap-card" data-i="' +
                i +
                '"><div class="cap-th"><img src="/api/capture/thumb?monitor=' +
                mo.index +
                "&w=360&t=" +
                Date.now() +
                '" onerror="this.style.visibility=\'hidden\'"></div>' +
                "<b>" +
                esc(mo.label) +
                "</b><span>" +
                mo.width +
                "×" +
                mo.height +
                "</span></button>",
            )
            .join("")
        : '<div class="empty" style="grid-column:1/-1">No screens found. Screen capture needs Windows 10 or later.</div>';
      $$(".cap-card", grid).forEach(
        (c) =>
          (c.onclick = () => {
            m.close();
            onPick({ monitor: data.monitors[+c.dataset.i] });
          }),
      );
    } else {
      grid.innerHTML = data.windows.length
        ? data.windows
            .map(
              (w, i) =>
                '<button class="cap-card" data-i="' +
                i +
                '"><div class="cap-th"><img loading="lazy" src="/api/capture/thumb?window=' +
                encodeURIComponent(w.hwnd) +
                "&w=360&t=" +
                Date.now() +
                '" onerror="this.style.visibility=\'hidden\'"></div>' +
                '<b title="' +
                esc(w.title) +
                '">' +
                esc(w.title) +
                "</b><span>" +
                esc(w.app.replace(/\.exe$/i, "")) +
                " · " +
                w.width +
                "×" +
                w.height +
                "</span></button>",
            )
            .join("")
        : '<div class="empty" style="grid-column:1/-1">No windows to capture. Open the app or browser first, then press Refresh.</div>';
      $$(".cap-card", grid).forEach(
        (c) =>
          (c.onclick = () => {
            m.close();
            onPick({ window: data.windows[+c.dataset.i] });
          }),
      );
    }
  };
  const load = async (fresh) => {
    try {
      data = await captureSources(fresh);
    } catch (e) {
      const g = $("#cp-grid");
      if (g) g.innerHTML = '<div class="empty" style="grid-column:1/-1">' + esc(e.message) + "</div>";
      return;
    }
    draw();
  };
  $$("#cp-tabs button").forEach(
    (b) =>
      (b.onclick = () => {
        tab = b.dataset.t;
        draw();
      }),
  );
  $("#cp-refresh").onclick = (e) => withSpinner(e.currentTarget, "", () => load(true));
  load(true);
}

function editSource(existing, kind) {
  kind = existing ? existing.kind : kind;
  const [W, H] = canvasSize();
  const s = existing
    ? Object.assign({}, existing)
    : {
        id: uid(),
        kind,
        x: kind === "text" ? 60 : 0,
        y: kind === "text" ? Math.round(H * 0.12) : 0,
        w: kind === "color" ? W : 0,
        h: kind === "color" ? H : 0,
        fit: "stretch",
        color: kind === "text" ? "white" : "black",
        text: "",
        font_size: 72,
      };
  if (existing) normalize(s);
  const swatches = ["white", "black", "#fe2c55", "#25f4ee", "#ffb13b", "#20d5a0", "#7c5cff", "#00ff00"];
  let body =
    '<div class="field"><label>Name <span style="color:var(--muted);font-weight:500">(optional)</span></label><input class="input" id="se-name" value="' +
    esc(s.name || "") +
    '" placeholder="' +
    esc(existing ? sourceTitle(existing) : KIND[kind].label) +
    '"></div>';
  if (kind === "video" || kind === "image") {
    body +=
      '<div class="field"><label>File</label><div class="row"><input class="input grow" id="se-path" spellcheck="false" value="' +
      esc(s.path || "") +
      '" readonly>' +
      '<button class="btn" id="se-browse">' +
      icon("folder") +
      "Change</button></div></div>";
  }
  if (kind === "video") {
    body += s.use_audio
      ? '<div class="field"><label>Sound on your LIVE</label><div class="row"><button class="icon-btn" id="se-mute" title="Mute"></button>' +
        '<input type="range" class="range grow" id="se-vol" min="0" max="1" step="0.01"><b class="ap-pct" id="se-pct" style="width:56px;text-align:right"></b></div></div>' +
        toggleRow(
          "se-hear",
          "Hear it in Studio",
          "Play the sound on this computer while you set up (only you hear it)",
          !!s.hear,
        )
      : '<p class="hint" style="margin:-4px 0 14px">This video has no sound.</p>';
  }
  if (kind === "screen") {
    body +=
      '<div class="field"><label>Capturing</label><div class="row"><div class="input grow cap-now" id="se-cap"></div><button class="btn" id="se-recap">' +
      icon("monitor") +
      "Change</button></div></div>";
    if (!s.window)
      body += toggleRow("se-cur", "Show mouse cursor", "Draw the pointer on the captured screen", !!s.cursor);
  }
  if (kind === "text") {
    body +=
      '<div class="field"><label>Text</label><textarea class="textarea prose" id="se-text" placeholder="Type your text">' +
      esc(s.text) +
      "</textarea></div>" +
      '<div class="field"><label>Size <span id="se-size-v" style="color:var(--muted)">' +
      s.font_size +
      '</span></label><input type="range" class="range" id="se-size" min="24" max="220" step="2" value="' +
      s.font_size +
      '"></div>';
  }
  if (kind === "color" || kind === "text") {
    body +=
      '<div class="field"><label>Color</label><div class="color-swatches" id="se-sw">' +
      swatches.map((c) => '<button data-c="' + c + '" style="background:' + c + '"></button>').join("") +
      '<input class="input" id="se-color" style="width:120px;height:30px" value="' +
      esc(s.color) +
      '"></div></div>';
  }
  if (kind !== "text") {
    body +=
      '<div class="section-title">Position &amp; size <span class="hint">canvas ' +
      W +
      "×" +
      H +
      "</span></div>" +
      '<div class="row xf-quick"><button class="btn sm" data-q="fit">Fit</button><button class="btn sm" data-q="fill">Fill</button><button class="btn sm" data-q="stretch">Stretch</button><button class="btn sm" data-q="center">Center</button></div>' +
      '<div class="grid4" style="margin-top:10px">' +
      ["x", "y", "w", "h"]
        .map(
          (k) =>
            '<div class="field"><label>' +
            k.toUpperCase() +
            '</label><input class="input" type="number" id="se-' +
            k +
            '" value="' +
            (s[k] | 0) +
            '"></div>',
        )
        .join("") +
      "</div>";
    if (canCrop(s)) {
      body +=
        '<div class="section-title">Crop <span class="hint" id="se-natinfo"></span></div><div class="grid4">' +
        [
          ["crop_l", "Left"],
          ["crop_t", "Top"],
          ["crop_r", "Right"],
          ["crop_b", "Bottom"],
        ]
          .map(
            ([k, l]) =>
              '<div class="field"><label>' +
              l +
              '</label><input class="input" type="number" min="0" id="se-' +
              k +
              '" value="' +
              (s[k] | 0) +
              '"></div>',
          )
          .join("") +
        "</div>" +
        '<div class="row"><label class="chk"><input type="checkbox" id="se-fh"' +
        (s.flip_h ? " checked" : "") +
        '> Flip horizontally</label><label class="chk"><input type="checkbox" id="se-fv"' +
        (s.flip_v ? " checked" : "") +
        "> Flip vertically</label></div>" +
        '<p class="hint" style="margin-top:8px">Tip: on the canvas, drag the corners to resize, hold <b>Alt</b> while dragging an edge to crop, and hold <b>Shift</b> to resize freely.</p>';
    }
  } else {
    body +=
      '<div class="grid2"><div class="field"><label>X</label><input class="input" type="number" id="se-x" value="' +
      (s.x | 0) +
      '"></div><div class="field"><label>Y</label><input class="input" type="number" id="se-y" value="' +
      (s.y | 0) +
      '"></div></div>';
  }

  const read = () => {
    s.name = $("#se-name").value.trim();
    if ($("#se-vol")) {
      s.volume = +$("#se-vol").value;
      s.hear = $("#se-hear").checked;
    }
    if ($("#se-cur")) s.cursor = $("#se-cur").checked;
    if ($("#se-text")) {
      s.text = $("#se-text").value;
      s.font_size = +$("#se-size").value;
    }
    if ($("#se-color")) s.color = $("#se-color").value.trim() || "white";
    ["x", "y", "w", "h", "crop_l", "crop_t", "crop_r", "crop_b"].forEach((k) => {
      if ($("#se-" + k)) s[k] = Math.round(+$("#se-" + k).value || 0);
    });
    ["crop_l", "crop_t", "crop_r", "crop_b"].forEach((k) => (s[k] = Math.max(0, s[k] | 0)));
    if ($("#se-fh")) {
      s.flip_h = $("#se-fh").checked;
      s.flip_v = $("#se-fv").checked;
    }
  };
  const write = () =>
    ["x", "y", "w", "h"].forEach((k) => {
      if ($("#se-" + k)) $("#se-" + k).value = s[k] | 0;
    });
  const m = openModal({
    title: (existing ? "Edit " : "Add ") + KIND[kind].label.toLowerCase(),
    size: "wide",
    body,
    foot: [
      {
        label: existing ? "Save" : "Add to scene",
        cls: "primary",
        fn: () => {
          read();
          if (kind === "text" && !s.text.trim()) return toast("Type some text", "err");
          if (canCrop(s) && s.nat_w && (s.crop_l + s.crop_r >= s.nat_w - 8 || s.crop_t + s.crop_b >= s.nat_h - 8))
            return toast("That crop removes the whole source", "err");
          if (kind !== "text" && ((s.w | 0) < 2 || (s.h | 0) < 2))
            return toast("Width and height must be at least 2", "err");
          const list = scene().sources;
          if (existing) list[list.indexOf(existing)] = s;
          else list.push(s);
          St.sel = s.id;
          saveScene();
          drawSources();
          renderSelBox();
          sceneChanged();
          syncHear();
          m.close();
        },
      },
    ],
  });
  if ($("#se-browse"))
    $("#se-browse").onclick = async () => {
      const p = await Native.pickFile(kind);
      if (!p) return;
      try {
        const d = await api("GET", "/api/media/info?path=" + encodeURIComponent(p));
        read();
        s.path = p;
        s.nat_w = d.width;
        s.nat_h = d.height;
        s.crop_l = s.crop_t = s.crop_r = s.crop_b = 0;
        $("#se-path").value = p;
        placeFit(s, "fit");
        write();
        natInfo();
        ["crop_l", "crop_t", "crop_r", "crop_b"].forEach((k) => $("#se-" + k) && ($("#se-" + k).value = 0));
      } catch (e) {
        toast(e.message, "err");
      }
    };
  const capLabel = () => {
    if ($("#se-cap"))
      $("#se-cap").textContent = s.window
        ? (s.window_title || "Window") + " · " + (s.window_app || "").replace(/\.exe$/i, "")
        : "Screen " + ((s.monitor | 0) + 1);
  };
  capLabel();
  if ($("#se-recap"))
    $("#se-recap").onclick = () =>
      openCapturePicker(s, (t) => {
        read();
        if (t.window)
          Object.assign(s, {
            window: t.window.hwnd,
            window_app: t.window.app,
            window_title: t.window.title,
            nat_w: t.window.width,
            nat_h: t.window.height,
          });
        else {
          Object.assign(s, { monitor: t.monitor.index, nat_w: t.monitor.width, nat_h: t.monitor.height });
          delete s.window;
          delete s.window_app;
          delete s.window_title;
        }
        s.crop_l = s.crop_t = s.crop_r = s.crop_b = 0;
        placeFit(s, "fit");
        write();
        capLabel();
        natInfo();
      });
  const natInfo = () => {
    if ($("#se-natinfo")) $("#se-natinfo").textContent = s.nat_w ? "source " + s.nat_w + "×" + s.nat_h + " px" : "";
  };
  if (canCrop(s)) ensureNat(s).then(natInfo);
  $$("[data-q]", m).forEach(
    (b) =>
      (b.onclick = async () => {
        read();
        if (b.dataset.q !== "stretch" && b.dataset.q !== "center" && !(await ensureNat(s)))
          return toast("Couldn't read this source's size", "err");
        if (b.dataset.q === "center") centerSrc(s);
        else placeFit(s, b.dataset.q);
        write();
      }),
  );
  if ($("#se-vol")) {
    const r = $("#se-vol"),
      mute = $("#se-mute");
    const draw = () => {
      mute.innerHTML = icon(audioIcon(s));
      mute.title = s.muted ? "Unmute" : "Mute";
      $("#se-pct").textContent = s.muted ? "Muted" : pct(+r.value);
      paintRange(r);
    };
    r.value = srcVolume(s);
    r.oninput = () => {
      s.muted = false;
      draw();
    };
    mute.onclick = () => {
      s.muted = !s.muted;
      draw();
    };
    draw();
  }
  if ($("#se-size")) {
    const r = $("#se-size");
    paintRange(r);
    r.oninput = () => {
      $("#se-size-v").textContent = r.value;
      paintRange(r);
    };
  }
  if ($("#se-sw"))
    $$("#se-sw button").forEach(
      (b) =>
        (b.onclick = () => {
          $("#se-color").value = b.dataset.c;
        }),
    );
}

const XF = { crop: false };

function selSource() {
  return scene().sources.find((x) => x.id === St.sel && !x.hidden);
}

function boxFor(s) {
  const [W, H] = canvasSize();
  if (s.kind === "text") {
    const ctx = (boxFor.c ||= document.createElement("canvas").getContext("2d"));
    const size = s.font_size || 72;
    ctx.font = "700 " + size + "px Segoe UI, Arial, sans-serif";
    const lines = String(s.text || "Text").split("\n");
    const w = Math.max(...lines.map((l) => ctx.measureText(l).width)) + 8;
    return { x: s.x | 0, y: s.y | 0, w, h: size * 1.25 * lines.length };
  }
  return { x: s.w ? s.x | 0 : 0, y: s.h ? s.y | 0 : 0, w: s.w || W, h: s.h || H };
}

const HANDLES = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
function renderSelBox() {
  const stage = $("#st-stage");
  if (!stage) return;
  $$(".sel-box", stage).forEach((x) => x.remove());
  const s = selSource();
  drawXfBar(s);
  if (!s || !St.scale) return;
  const b = boxFor(s),
    k = St.scale;
  const hs = s.kind === "text" ? ["se"] : HANDLES;
  const box = node(
    '<div class="sel-box' +
      (XF.crop && canCrop(s) ? " cropping" : "") +
      '"><span class="lbl">' +
      esc(sourceTitle(s)) +
      "</span>" +
      hs.map((h) => '<i class="hd h-' + h + '" data-h="' + h + '"></i>').join("") +
      "</div>",
  );
  Object.assign(box.style, {
    left: b.x * k + "px",
    top: b.y * k + "px",
    width: b.w * k + "px",
    height: b.h * k + "px",
  });
  stage.appendChild(box);
}

function drawXfBar(s) {
  const bar = $("#xf-bar");
  if (!bar) return;
  if (!s) {
    bar.hidden = true;
    return;
  }
  bar.hidden = false;
  const media = XF_KINDS.includes(s.kind);
  // main actions get labels, the rest are icon-only to keep the bar short
  const btn = (a, ic, label, on, iconOnly) =>
    '<button class="xf-b' +
    (on ? " on" : "") +
    (iconOnly ? " io" : "") +
    '" data-a="' +
    a +
    '" title="' +
    label +
    '">' +
    icon(ic) +
    (iconOnly ? "" : "<span>" + label + "</span>") +
    "</button>";
  bar.innerHTML =
    (media ? btn("fit", "min", "Fit") + btn("fill", "max", "Fill") + btn("stretch", "grid", "Stretch") : "") +
    btn("center", "target", "Center", false, media) +
    (canCrop(s)
      ? '<span class="xf-sep"></span>' +
        btn("crop", "crop", "Crop", XF.crop) +
        (s.crop_l | s.crop_t | s.crop_r | s.crop_b ? btn("uncrop", "refresh", "Reset crop", false, true) : "") +
        btn("fliph", "fliph", "Flip horizontally", s.flip_h, true)
      : "") +
    '<span class="xf-sep"></span>' +
    btn("edit", "edit", "Edit…", false, true) +
    btn("del", "trash", "Remove", false, true);
  $$("[data-a]", bar).forEach((b) => (b.onclick = () => xfAction(b.dataset.a)));
}

async function xfAction(a) {
  const s = selSource();
  if (!s) return;
  if (a === "edit") return editSource(s);
  if (a === "del") {
    const list = scene().sources;
    list.splice(list.indexOf(s), 1);
    St.sel = null;
    saveScene();
    drawSources();
    renderSelBox();
    sceneChanged();
    return;
  }
  if (a === "crop") {
    XF.crop = !XF.crop;
    renderSelBox();
    if (XF.crop) toast("Drag an edge or corner to crop. Press Crop again when you're done.");
    return;
  }
  if (!(await ensureNat(s)) && (a === "fit" || a === "fill")) return toast("Couldn't read this source's size", "err");
  normalize(s);
  if (a === "fit" || a === "fill" || a === "stretch") placeFit(s, a);
  else if (a === "center") centerSrc(s);
  else if (a === "uncrop") {
    // grow the box back so the content stays the same size
    const [cw, ch] = cropped(s),
      kx = s.w / cw,
      ky = s.h / ch;
    s.x -= Math.round((s.crop_l | 0) * kx);
    s.y -= Math.round((s.crop_t | 0) * ky);
    s.w += Math.round(((s.crop_l | 0) + (s.crop_r | 0)) * kx);
    s.h += Math.round(((s.crop_t | 0) + (s.crop_b | 0)) * ky);
    s.crop_l = s.crop_t = s.crop_r = s.crop_b = 0;
  } else if (a === "fliph") s.flip_h = !s.flip_h;
  saveScene();
  renderSelBox();
  sceneChanged();
}

// Snap the box's edges and centre to the canvas edges and centre (within 8
// screen pixels).
function snapBox(b, guides) {
  const [W, H] = canvasSize(),
    th = 8 / St.scale;
  const best = (vals, lines) => {
    let hit = null;
    for (const [v, off] of vals)
      for (const L of lines) {
        const d = L - v;
        if (Math.abs(d) < th && (!hit || Math.abs(d) < Math.abs(hit.d))) hit = { d, L, off };
      }
    return hit;
  };
  const hx = best(
    [
      [b.x, 0],
      [b.x + b.w / 2, 0],
      [b.x + b.w, 0],
    ],
    [0, W / 2, W],
  );
  const hy = best(
    [
      [b.y, 0],
      [b.y + b.h / 2, 0],
      [b.y + b.h, 0],
    ],
    [0, H / 2, H],
  );
  if (hx) {
    b.x += hx.d;
    guides.v = hx.L;
  }
  if (hy) {
    b.y += hy.d;
    guides.h = hy.L;
  }
}
function drawGuides(g) {
  const stage = $("#st-stage");
  $$(".guide", stage).forEach((x) => x.remove());
  if (!g) return;
  const k = St.scale;
  if (g.v != null) stage.appendChild(node('<div class="guide gv" style="left:' + g.v * k + 'px"></div>'));
  if (g.h != null) stage.appendChild(node('<div class="guide gh" style="top:' + g.h * k + 'px"></div>'));
}

// While LIVE the canvas is the stream, so during a drag we move a snapshot of
// the layer taken from the current frame.
function ghostFor(b) {
  const img = $("#pv-frame");
  if (!img || img.hidden || !img.naturalWidth) return null;
  const [W] = canvasSize(),
    k = img.naturalWidth / W;
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(b.w * k));
  c.height = Math.max(1, Math.round(b.h * k));
  try {
    c.getContext("2d").drawImage(img, b.x * k, b.y * k, b.w * k, b.h * k, 0, 0, c.width, c.height);
  } catch {
    return null;
  }
  c.className = "ghost";
  return c;
}

function wireStage() {
  const stage = $("#st-stage");
  stage.addEventListener("pointerdown", async (e) => {
    if (e.button !== 0) return;
    const rect = stage.getBoundingClientRect(),
      k = St.scale;
    const cx = (e.clientX - rect.left) / k,
      cy = (e.clientY - rect.top) / k;
    const handle = e.target.dataset.h || null;
    let s = selSource();
    if (!handle && !e.target.closest(".sel-box")) {
      // pick the top-most layer under the pointer
      s = visibleSources()
        .slice()
        .reverse()
        .find((x) => {
          const b = boxFor(x);
          return cx >= b.x && cx <= b.x + b.w && cy >= b.y && cy <= b.y + b.h;
        });
      St.sel = s ? s.id : null;
      if (!s) XF.crop = false;
      drawSources();
      renderSelBox();
      if (!s) return;
    }
    const cropping = !!handle && canCrop(s) && (XF.crop || e.altKey);
    if (cropping || (handle && s.kind !== "text")) {
      await ensureNat(s);
      normalize(s);
      renderSelBox();
    }
    const start = boxFor(s),
      st = {
        crop_l: s.crop_l | 0,
        crop_t: s.crop_t | 0,
        crop_r: s.crop_r | 0,
        crop_b: s.crop_b | 0,
        font: s.font_size || 72,
      };
    const [cw0, ch0] = cropped(s),
      pxX = s.nat_w ? cw0 / start.w : 1,
      pxY = s.nat_h ? ch0 / start.h : 1;
    const box = $(".sel-box", stage);
    if (!box) return;
    const ghost = !editorMode() && !cropping && s.kind !== "text" ? ghostFor(start) : null;
    const layer = editorMode() ? layerEl(s.id) : null;
    const sx = e.clientX,
      sy = e.clientY;
    let moved = false,
      draft = null;
    try {
      stage.setPointerCapture(e.pointerId);
    } catch {}

    const move = (ev) => {
      const dx = (ev.clientX - sx) / k,
        dy = (ev.clientY - sy) / k;
      if (!moved && Math.abs(dx) + Math.abs(dy) < 2 / k) return;
      if (!moved) {
        moved = true;
        if (ghost) box.appendChild(ghost);
      }
      let b = Object.assign({}, start);
      const guides = {};
      const c = Object.assign({}, st);
      if (!handle) {
        b.x = start.x + dx;
        b.y = start.y + dy;
        if (!ev.altKey) snapBox(b, guides);
      } else if (cropping) {
        const minW = 16 / pxX,
          minH = 16 / pxY;
        if (handle.includes("w")) {
          const d = Math.min(Math.max(dx, -st.crop_l / pxX), start.w - minW);
          b.x = start.x + d;
          b.w = start.w - d;
          c.crop_l = Math.round(st.crop_l + d * pxX);
        }
        if (handle.includes("e")) {
          const d = Math.min(Math.max(-dx, -st.crop_r / pxX), start.w - minW);
          b.w = start.w - d;
          c.crop_r = Math.round(st.crop_r + d * pxX);
        }
        if (handle.includes("n")) {
          const d = Math.min(Math.max(dy, -st.crop_t / pxY), start.h - minH);
          b.y = start.y + d;
          b.h = start.h - d;
          c.crop_t = Math.round(st.crop_t + d * pxY);
        }
        if (handle.includes("s")) {
          const d = Math.min(Math.max(-dy, -st.crop_b / pxY), start.h - minH);
          b.h = start.h - d;
          c.crop_b = Math.round(st.crop_b + d * pxY);
        }
      } else if (s.kind === "text") {
        const f = Math.max(0.2, (start.h + dy) / start.h);
        c.font = Math.max(16, Math.round(st.font * f));
        b.w = start.w * f;
        b.h = start.h * f;
      } else {
        if (handle.includes("e")) b.w = start.w + dx;
        if (handle.includes("w")) b.w = start.w - dx;
        if (handle.includes("s")) b.h = start.h + dy;
        if (handle.includes("n")) b.h = start.h - dy;
        if (handle.length === 2 && !ev.shiftKey) {
          // corners keep the aspect ratio
          const r = start.w / start.h;
          if (Math.abs(b.w / start.w - 1) > Math.abs(b.h / start.h - 1)) b.h = b.w / r;
          else b.w = b.h * r;
        }
        b.w = Math.max(16, b.w);
        b.h = Math.max(16, b.h);
        if (handle.includes("w")) b.x = start.x + start.w - b.w;
        if (handle.includes("n")) b.y = start.y + start.h - b.h;
      }
      draft = { b, c };
      Object.assign(box.style, {
        left: b.x * k + "px",
        top: b.y * k + "px",
        width: b.w * k + "px",
        height: b.h * k + "px",
      });
      if (layer)
        positionLayer(
          layer,
          s.kind === "text"
            ? Object.assign({}, s, { x: b.x, y: b.y, font_size: c.font })
            : Object.assign(
                {},
                s,
                { x: b.x, y: b.y, w: b.w, h: b.h, fit: "stretch" },
                cropping ? { crop_l: c.crop_l, crop_t: c.crop_t, crop_r: c.crop_r, crop_b: c.crop_b } : {},
              ),
        );
      drawGuides(guides);
    };
    const up = () => {
      stage.removeEventListener("pointermove", move);
      stage.removeEventListener("pointerup", up);
      stage.removeEventListener("pointercancel", up);
      drawGuides(null);
      if (!moved || !draft) {
        renderSelBox();
        return;
      }
      const { b, c } = draft;
      if (s.kind === "text") {
        s.x = Math.round(b.x);
        s.y = Math.round(b.y);
        s.font_size = c.font;
      } else {
        Object.assign(s, { x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.w), h: Math.round(b.h) });
        if (cropping) Object.assign(s, { crop_l: c.crop_l, crop_t: c.crop_t, crop_r: c.crop_r, crop_b: c.crop_b });
        if (!s.fit || s.fit !== "stretch") normalize(s);
      }
      saveScene();
      renderSelBox();
      sceneChanged();
    };
    stage.addEventListener("pointermove", move);
    stage.addEventListener("pointerup", up);
    stage.addEventListener("pointercancel", up);
  });
}

// Studio keys: arrows nudge, Delete removes, Esc deselects. Ignored while typing.
function studioKeys(e) {
  if (S.view !== "studio" || modals.length || e.target.closest("input, textarea, select, [contenteditable]")) return;
  const s = selSource();
  if (!s) return;
  const step = e.shiftKey ? 10 : 1,
    d = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }[e.key];
  if (d) {
    e.preventDefault();
    if (s.kind !== "text") normalize(s);
    const b = boxFor(s);
    s.x = Math.round(b.x + d[0]);
    s.y = Math.round(b.y + d[1]);
    if (s.kind !== "text" && !s.w) {
      s.w = b.w;
      s.h = b.h;
    }
    saveScene();
    renderSelBox();
    sceneChangedSoon();
  } else if (e.key === "Delete" || e.key === "Backspace") {
    e.preventDefault();
    xfAction("del");
  } else if (e.key === "Escape") {
    St.sel = null;
    XF.crop = false;
    drawSources();
    renderSelBox();
  }
}
const sceneChangedSoon = debounce(() => sceneChanged(), 400);
