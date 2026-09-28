// First-run setup: loads the machine's devices and downloads ffmpeg if it
// isn't installed.
"use strict";

const Setup = { modal: null, timer: 0 };

async function loadDevices() {
  try {
    const d = await api("GET", "/api/devices");
    S.devices = Object.assign({}, S.devices, d, { loaded: true });
  } catch {
    S.devices.loaded = true;
  }
  const v = VIEWS[S.view];
  if (v && v.devices) v.devices();
}

// Called once devices are known. Without ffmpeg nothing can stream, so the
// download starts right away.
function checkFFmpeg() {
  if (S.devices.loaded && !S.devices.ffmpeg) openFFmpegSetup();
}

async function openFFmpegSetup() {
  if (Setup.modal) return;
  Setup.modal = openModal({
    title: "Installing ffmpeg",
    sub: "Lasted Live uses ffmpeg to encode and send your stream to TikTok. It's a one-time download of about 110 MB.",
    body:
      '<div class="setup-bar"><i id="fs-fill"></i></div>' +
      '<div class="setup-line"><span id="fs-text">Starting…</span><span id="fs-size"></span></div>',
    foot: [
      { label: "Hide", cls: "ghost", fn: () => Setup.modal && Setup.modal.close() },
      { label: "Try again", cls: "primary", fn: () => startFFmpegSetup() },
    ],
    onClose: () => {
      Setup.modal = null;
    },
  });
  $(".modal-foot .primary", Setup.modal).hidden = true;
  await startFFmpegSetup();
}

async function startFFmpegSetup() {
  try {
    drawFFmpegSetup(await api("POST", "/api/ffmpeg/setup"));
  } catch (e) {
    drawFFmpegSetup({ state: "error", error: e.message });
    return;
  }
  clearInterval(Setup.timer);
  Setup.timer = setInterval(pollFFmpegSetup, 500);
}

async function pollFFmpegSetup() {
  let st;
  try {
    st = await api("GET", "/api/ffmpeg/setup");
  } catch {
    return;
  }
  drawFFmpegSetup(st);
  if (st.state === "done" || st.state === "error") clearInterval(Setup.timer);
  if (st.state === "done") {
    await loadDevices();
    toast("ffmpeg is installed. You're ready to go LIVE.", "ok");
    if (Setup.modal) Setup.modal.close();
  }
}

function drawFFmpegSetup(st) {
  if (!Setup.modal) {
    if (st.state === "error") toast("ffmpeg install failed: " + st.error, "err");
    return;
  }
  const mb = (n) => (n / 1048576).toFixed(0) + " MB";
  const pct = st.total > 0 ? Math.min(100, (st.done / st.total) * 100) : 0;
  const text = {
    downloading: "Downloading…",
    verifying: "Checking the download…",
    extracting: "Unpacking…",
    done: "Done",
    error: st.error || "Something went wrong",
  }[st.state];
  $("#fs-fill").style.width = (st.state === "downloading" ? pct : st.state === "error" ? 0 : 100) + "%";
  $("#fs-text").textContent = text || "Starting…";
  $("#fs-text").classList.toggle("bad-t", st.state === "error");
  $("#fs-size").textContent = st.state === "downloading" && st.total > 0 ? mb(st.done) + " / " + mb(st.total) : "";
  $(".modal-foot .primary", Setup.modal).hidden = st.state !== "error";
}
