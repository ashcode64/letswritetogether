const pagesEl = document.querySelector("#pages");
const openedEl = document.querySelector("#opened");
const filesEl = document.querySelector("#files");
const listEl = document.querySelector("#list");
const entry = document.querySelector("#entry");
const send = document.querySelector("#send");
const note = document.querySelector("#note");
const submitLeft = document.querySelector("#submit-left");
const voteLeft = document.querySelector("#vote-left");
const submitClock = document.querySelector("#submit-clock");
const search = document.querySelector("#search");

const state = {
  offset: 0,
  roundId: 0,
  phase: "vote",
  seed: 1,
  words: [],
  order: [],
  mine: null,
  vote: null,
  fragments: [],
  view: "",
  today: "",
  essays: new Map(),
};

let accessToken = "";
let accessUntil = 0;

function b64url(buf) {
  const bytes = new Uint8Array(buf);
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function idb() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open("lwt-device", 1);
    req.onupgradeneeded = () => req.result.createObjectStore("keys");
    req.onerror = () => reject(req.error);
    req.onsuccess = () => resolve(req.result);
  });
}

function idbGet(db, key) {
  return new Promise((resolve, reject) => {
    const q = db.transaction("keys").objectStore("keys").get(key);
    q.onsuccess = () => resolve(q.result || null);
    q.onerror = () => reject(q.error);
  });
}

function idbPut(db, key, value) {
  return new Promise((resolve, reject) => {
    const q = db.transaction("keys", "readwrite").objectStore("keys").put(value, key);
    q.onsuccess = () => resolve();
    q.onerror = () => reject(q.error);
  });
}

async function deviceKey(forceNew) {
  const db = await idb();
  if (!forceNew) {
    const existing = await idbGet(db, "device");
    if (existing) return existing;
  }
  const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, false, ["sign"]);
  await idbPut(db, "device", pair);
  return pair;
}

async function prove(pair, msg) {
  const sig = await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, pair.privateKey, new TextEncoder().encode(msg));
  return b64url(sig);
}

async function publicJWK(pair) {
  const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  return { kty: jwk.kty, crv: jwk.crv, x: jwk.x, y: jwk.y };
}

function keepAccess(body) {
  accessToken = body.access_token;
  accessUntil = Date.now() + (body.expires_in - 20) * 1000;
}

async function raw(path, opt = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    ...opt,
    headers: { "Content-Type": "application/json", ...(opt.headers || {}) },
  });
  const text = await res.text();
  const body = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const err = new Error(body && body.detail ? body.detail : res.statusText);
    err.body = body;
    err.status = res.status;
    throw err;
  }
  return body;
}

const powSource = [
  "function rotr(x, n) { return ((x >>> n) | (x << (32 - n))) >>> 0; }",
  "function sha256Hex(str) {",
  "  const K = [",
  "    0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,",
  "    0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,",
  "    0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,",
  "    0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,",
  "    0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,",
  "    0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,",
  "    0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,",
  "    0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2",
  "  ];",
  "  const bytes = new TextEncoder().encode(str);",
  "  const bitLen = bytes.length * 8;",
  "  const withOne = bytes.length + 1;",
  "  const pad = (64 - ((withOne + 8) % 64)) % 64;",
  "  const data = new Uint8Array(withOne + pad + 8);",
  "  data.set(bytes);",
  "  data[bytes.length] = 0x80;",
  "  const view = new DataView(data.buffer);",
  "  view.setUint32(data.length - 8, Math.floor(bitLen / 0x100000000));",
  "  view.setUint32(data.length - 4, bitLen >>> 0);",
  "  let h0=0x6a09e667,h1=0xbb67ae85,h2=0x3c6ef372,h3=0xa54ff53a,h4=0x510e527f,h5=0x9b05688c,h6=0x1f83d9ab,h7=0x5be0cd19;",
  "  const w = new Uint32Array(64);",
  "  for (let i = 0; i < data.length; i += 64) {",
  "    for (let t = 0; t < 16; t++) w[t] = view.getUint32(i + t * 4);",
  "    for (let t = 16; t < 64; t++) {",
  "      const s0 = (rotr(w[t-15],7) ^ rotr(w[t-15],18) ^ (w[t-15] >>> 3)) >>> 0;",
  "      const s1 = (rotr(w[t-2],17) ^ rotr(w[t-2],19) ^ (w[t-2] >>> 10)) >>> 0;",
  "      w[t] = (w[t-16] + s0 + w[t-7] + s1) >>> 0;",
  "    }",
  "    let a=h0,b=h1,c=h2,d=h3,e=h4,f=h5,g=h6,h=h7;",
  "    for (let t = 0; t < 64; t++) {",
  "      const S1 = (rotr(e,6) ^ rotr(e,11) ^ rotr(e,25)) >>> 0;",
  "      const ch = ((e & f) ^ ((~e) & g)) >>> 0;",
  "      const t1 = (h + S1 + ch + K[t] + w[t]) >>> 0;",
  "      const S0 = (rotr(a,2) ^ rotr(a,13) ^ rotr(a,22)) >>> 0;",
  "      const maj = ((a & b) ^ (a & c) ^ (b & c)) >>> 0;",
  "      const t2 = (S0 + maj) >>> 0;",
  "      h=g; g=f; f=e; e=(d + t1) >>> 0; d=c; c=b; b=a; a=(t1 + t2) >>> 0;",
  "    }",
  "    h0=(h0+a)>>>0; h1=(h1+b)>>>0; h2=(h2+c)>>>0; h3=(h3+d)>>>0;",
  "    h4=(h4+e)>>>0; h5=(h5+f)>>>0; h6=(h6+g)>>>0; h7=(h7+h)>>>0;",
  "  }",
  "  return [h0,h1,h2,h3,h4,h5,h6,h7].map((x) => x.toString(16).padStart(8, '0')).join('');",
  "}",
  "self.onmessage = (e) => {",
  "  const salt = e.data.salt, challenge = e.data.challenge, max = e.data.max;",
  "  for (let n = 0; n <= max; n++) {",
  "    if (sha256Hex(salt + n) === challenge) { self.postMessage(n); return; }",
  "  }",
  "  self.postMessage(-1);",
  "};",
].join("\n");

function solvePow(ch) {
  return new Promise((resolve, reject) => {
    const worker = new Worker(URL.createObjectURL(new Blob([powSource], { type: "text/javascript" })));
    const fail = () => {
      worker.terminate();
      reject(new Error("Proof of work failed."));
    };
    worker.onmessage = (e) => {
      worker.terminate();
      if (e.data < 0) fail();
      else resolve(e.data);
    };
    worker.onerror = fail;
    worker.postMessage({ salt: ch.salt, challenge: ch.challenge, max: ch.maxnumber });
  });
}

function loadTurnstile() {
  if (window.turnstile && typeof window.turnstile.render === "function") return Promise.resolve();
  return new Promise((resolve, reject) => {
    const ready = () => window.turnstile && typeof window.turnstile.render === "function";
    if (ready()) {
      resolve();
      return;
    }
    const fail = () => reject(new Error("Could not reach the human check."));
    const wait = () => {
      const start = Date.now();
      const timer = setInterval(() => {
        if (ready()) {
          clearInterval(timer);
          resolve();
        } else if (Date.now() - start > 8000) {
          clearInterval(timer);
          fail();
        }
      }, 50);
    };
    if (document.querySelector("script[data-turnstile]")) {
      wait();
      return;
    }
    window.onTurnstile = () => {};
    const s = document.createElement("script");
    s.dataset.turnstile = "1";
    s.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=onTurnstile";
    s.async = true;
    s.onerror = fail;
    document.head.appendChild(s);
    wait();
  });
}

function turnstileToken(sitekey) {
  return loadTurnstile().then(() => new Promise((resolve, reject) => {
    const el = document.querySelector("#turnstile");
    el.replaceChildren();
    window.turnstile.render(el, {
      sitekey,
      appearance: "interaction-only",
      callback: resolve,
      "error-callback": () => reject(new Error("Human check failed.")),
    });
  }));
}

async function refresh(pair) {
  const ts = Math.floor(Date.now() / 1000);
  try {
    const body = await raw("/v1/sessions/refresh", {
      method: "POST",
      body: JSON.stringify({ ts, proof: await prove(pair, "refresh\n" + ts) }),
    });
    keepAccess(body);
    return true;
  } catch (err) {
    if (err.body && err.body.code === "DEVICE_REVOKED") throw err;
    if (err.status === 401) return false;
    throw err;
  }
}

async function mint(pair) {
  const cfg = await raw("/v1/sessions/config");
  const ch = await raw("/v1/sessions/challenge", { method: "POST", body: "{}" });
  const solution = await solvePow(ch);
  const turnstileTokenValue = await turnstileToken(cfg.turnstile_site_key);
  const proof = await prove(pair, "mint\n" + ch.signature);
  try {
    const body = await raw("/v1/sessions", {
      method: "POST",
      body: JSON.stringify({
        public_key: await publicJWK(pair),
        salt: ch.salt,
        challenge: ch.challenge,
        maxnumber: ch.maxnumber,
        expires: ch.expires,
        signature: ch.signature,
        solution,
        proof,
        turnstile_token: turnstileTokenValue,
      }),
    });
    keepAccess(body);
  } catch (err) {
    if (err.body && err.body.code === "DEVICE_REVOKED") {
      await mint(await deviceKey(true));
      return;
    }
    throw err;
  }
}

async function restoreSession() {
  const pair = await deviceKey(false);
  try {
    if (await refresh(pair)) return;
  } catch (err) {
    if (err.body && err.body.code === "DEVICE_REVOKED") {
      await mint(await deviceKey(true));
      return;
    }
    throw err;
  }
  await mint(pair);
}

function serverNow() { return Date.now() + state.offset; }

function utcDay(ms) { return new Date(ms).toISOString().slice(0, 10); }

async function api(path, opt = {}) {
  const retry = opt.retry !== false;
  if (!accessToken || Date.now() >= accessUntil) await restoreSession();
  const headers = { "Content-Type": "application/json", ...(opt.headers || {}) };
  if (accessToken) headers.Authorization = "Bearer " + accessToken;
  const res = await fetch(path, {
    credentials: "same-origin",
    method: opt.method,
    body: opt.body,
    headers,
  });
  const text = await res.text();
  const body = text ? JSON.parse(text) : null;
  if (res.status === 401 && retry) {
    accessToken = "";
    await restoreSession();
    return api(path, { ...opt, retry: false });
  }
  if (!res.ok) {
    const err = new Error(body && body.detail ? body.detail : res.statusText);
    err.body = body;
    throw err;
  }
  return body;
}

function applySnap(s) {
  state.offset = s.server_time_ms - Date.now();
  const changed = s.round_id !== state.roundId;
  state.roundId = s.round_id;
  state.phase = s.phase;
  if (s.shuffle_seed) state.seed = s.shuffle_seed;
  state.today = utcDay(s.server_time_ms);
  if (!state.view) state.view = state.today;
  return changed;
}

function mulberry32(a) {
  return function () {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function place(word) {
  if (state.order.some((w) => w.id === word.id)) return;
  const rand = mulberry32((state.seed + Number(word.id || 1)) >>> 0);
  const i = Math.floor(rand() * (state.order.length + 1));
  state.order.splice(i, 0, word);
}

function hasLetter(s) {
  return /[A-Za-z]/.test(s);
}

function needsSpace(prev, next) {
  if (!next || !hasLetter(next)) return false;
  if (".,!?;:…)\]}".includes(next[0]) || next.startsWith("…")) return false;
  if (!prev) return true;
  const last = prev[prev.length - 1];
  return !"([{\"'“‘".includes(last);
}

function join(parts) {
  let out = "";
  parts.forEach((p, i) => {
    if (i > 0 && needsSpace(parts[i - 1], p)) out += " ";
    out += p;
  });
  return out;
}

function openDialog() {
  document.querySelector("#box").showModal();
}

function renderEssay() {
  const text = join(state.fragments);
  const blank = !text;
  paintPages(blank ? "The page is blank. The first word has not been chosen yet. " : text, {
    blank,
    slot: state.view === state.today,
    leadingSpace: !blank && needsSpace(state.fragments[state.fragments.length - 1] || "", "word"),
  });
  const past = state.view && state.view !== state.today;
  openedEl.hidden = !past;
  openedEl.textContent = past ? fileDate(state.view) : "";
}

function makePage() {
  const page = document.createElement("div");
  page.className = "page";
  const essay = document.createElement("article");
  essay.className = "essay";
  page.append(essay);
  return { page, essay };
}

function contentMetrics() {
  const { page, essay } = makePage();
  pagesEl.append(page);
  const cs = getComputedStyle(page);
  const height = page.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom);
  const width = essay.clientWidth;
  page.remove();
  return { width, height };
}

function fitEnd(node, start, height) {
  const len = node.length;
  if (start >= len) return len;
  const range = document.createRange();
  let lo = start;
  let hi = len;
  range.setStart(node, start);
  range.setEnd(node, Math.min(start + 1, len));
  const first = range.getClientRects();
  if (!first.length) return Math.min(start + 1, len);
  const top = first[0].top;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    range.setStart(node, start);
    range.setEnd(node, mid);
    const rects = range.getClientRects();
    const bottom = rects.length ? rects[rects.length - 1].bottom : top;
    if (bottom - top <= height + 0.5) lo = mid;
    else hi = mid - 1;
  }
  return Math.max(lo, start);
}

function textPages(text, width, height) {
  if (!text) return [""];
  const measure = document.createElement("div");
  measure.className = "essay";
  measure.style.cssText = "position:absolute;left:0;top:0;visibility:hidden;pointer-events:none;";
  measure.style.width = width + "px";
  measure.textContent = text;
  document.body.append(measure);
  const node = measure.firstChild;
  const pages = [];
  let start = 0;
  while (start < text.length && pages.length < 80) {
    const end = fitEnd(node, start, height);
    const next = Math.max(end, start + 1);
    let chunk = text.slice(start, next);
    if (pages.length) chunk = chunk.replace(/^\s+/, "");
    pages.push(chunk);
    start = next;
  }
  measure.remove();
  return pages.length ? pages : [""];
}

function makeSlot() {
  const slot = document.createElement("button");
  slot.type = "button";
  slot.className = "type-slot";
  slot.setAttribute("aria-label", "Type the next word");
  return slot;
}

function paintPages(text, opt) {
  pagesEl.replaceChildren();
  const { width, height } = contentMetrics();
  const chunks = opt.blank ? [text] : textPages(text, width, height);
  chunks.forEach((chunk, i) => {
    const { page, essay } = makePage();
    if (opt.blank) {
      const hint = document.createElement("span");
      hint.className = "blank";
      hint.textContent = chunk;
      essay.append(hint);
    } else if (chunk) {
      essay.append(document.createTextNode(chunk));
    }
    if (i === chunks.length - 1 && opt.slot) {
      if (opt.leadingSpace && chunk) essay.append(document.createTextNode(" "));
      essay.append(makeSlot());
      pagesEl.append(page);
      if (essay.scrollHeight > height + 1) {
        const space = essay.lastChild.previousSibling;
        essay.removeChild(essay.lastChild);
        if (space && space.nodeType === Node.TEXT_NODE && space.textContent === " ") essay.removeChild(space);
        const extra = makePage();
        if (opt.leadingSpace) extra.essay.append(document.createTextNode(" "));
        extra.essay.append(makeSlot());
        pagesEl.append(extra.page);
      }
    } else {
      pagesEl.append(page);
    }
  });
}

function fileDate(iso) {
  const [y, m, d] = iso.split("-");
  return d + "-" + m + "-" + y;
}

const pastBtn = document.querySelector("#past");

function showSheet() {
  document.body.classList.remove("shelf");
  pastBtn.textContent = "Past submissions";
}

function backToToday() {
  if (location.hash) history.pushState({ shelf: false }, "", location.pathname);
  state.view = state.today;
  showSheet();
  loadEssay(state.today);
}

async function showShelf() {
  document.body.classList.add("shelf");
  pastBtn.textContent = "today's document";
  filesEl.replaceChildren();
  let days = [];
  try {
    const body = await api("/v1/canvas/days");
    days = body.days || [];
  } catch {
    days = [];
  }
  if (!document.body.classList.contains("shelf")) return;
  filesEl.replaceChildren();
  if (!days.length) {
    const empty = document.createElement("p");
    empty.className = "shelf-empty";
    empty.textContent = "No day has a word yet.";
    filesEl.append(empty);
    return;
  }
  for (const day of days) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "file";
    btn.dataset.day = day;
    btn.setAttribute("aria-label", fileDate(day));
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 128 164");
    svg.setAttribute("aria-hidden", "true");
    const page = document.createElementNS("http://www.w3.org/2000/svg", "path");
    page.setAttribute("d", "M2 2 H98 L126 30 V162 H2 Z");
    page.setAttribute("fill", "#f3e6c8");
    page.setAttribute("stroke", "#6b5438");
    page.setAttribute("stroke-width", "2");
    const fold = document.createElementNS("http://www.w3.org/2000/svg", "path");
    fold.setAttribute("d", "M98 2 V30 H126 Z");
    fold.setAttribute("fill", "#e4d0a2");
    fold.setAttribute("stroke", "#6b5438");
    fold.setAttribute("stroke-width", "2");
    svg.append(page, fold);
    const label = document.createElement("span");
    label.className = "file-date";
    label.textContent = fileDate(day);
    btn.append(svg, label);
    filesEl.append(btn);
  }
}

const votingNote = "Voting only, submit a word in the first 30 seconds of the next minute";

function setNote(text, kind) {
  note.textContent = text;
  if (kind) note.dataset.kind = kind;
  else delete note.dataset.kind;
  note.classList.toggle("phase", kind === "phase");
}

function showPhaseNote() {
  if (state.phase === "submit") {
    if (note.dataset.kind === "phase") setNote("");
    return;
  }
  if (note.dataset.kind !== "error") setNote(votingNote, "phase");
}

function renderClock() {
  const ms = serverNow();
  const sec = Math.floor(ms / 1000);
  const within = ((sec % 60) + 60) % 60;
  const rid = Math.floor(sec / 60);
  const ph = within < 30 ? "submit" : "vote";
  if (rid !== state.roundId) {
    state.roundId = rid;
    state.phase = ph;
    state.words = [];
    state.order = [];
    state.mine = null;
    state.vote = null;
    loadWords();
  } else {
    state.phase = ph;
  }
  submitLeft.textContent = Math.max(0, 30 - within) + "s";
  voteLeft.textContent = (60 - within) + "s";
  submitClock.classList.toggle("closed", ph !== "submit");
  const open = ph === "submit";
  entry.disabled = !open;
  send.disabled = !open;
  entry.placeholder = "Write a word";
  showPhaseNote();
}

function renderList() {
  const q = search.value;
  const rows = state.order.filter((w) => !q || w.text.includes(q));
  listEl.innerHTML = "";
  for (const w of rows) {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "word";
    const label = document.createElement("span");
    label.textContent = w.text;
    btn.append(label);
    if (w.id === state.mine) {
      li.classList.add("mine");
      const tag = document.createElement("span");
      tag.className = "tag";
      tag.textContent = "yours";
      btn.append(tag);
    }
    if (w.id === state.vote) {
      li.classList.add("selected");
      const star = document.createElement("span");
      star.className = "star";
      star.textContent = "★";
      star.setAttribute("aria-label", "your vote");
      btn.append(star);
    }
    btn.addEventListener("click", () => choose(w));
    li.append(btn);
    listEl.appendChild(li);
  }
}

async function searchNow() {
  if (note.dataset.kind === "error") setNote("");
  try {
    const body = await api("/v1/rounds/" + state.roundId + "/words");
    for (const w of body.words || []) place(w);
    renderList();
  } catch (err) {
    setNote(err.message, "error");
  }
}

async function loadEssay(day) {
  if (state.essays.has(day)) {
    state.fragments = state.essays.get(day).slice();
    renderEssay();
    return;
  }
  const path = day === state.today ? "/v1/canvas/today" : "/v1/canvas/" + day;
  let fragments = [];
  try {
    const body = await api(path);
    fragments = body.fragments || [];
  } catch {
    fragments = [];
  }
  state.essays.set(day, fragments);
  state.fragments = fragments.slice();
  renderEssay();
}

async function loadWords() {
  const body = await api("/v1/rounds/" + state.roundId + "/words");
  state.words = body.words || [];
  state.order = [];
  const copy = state.words.slice();
  const rand = mulberry32(state.seed >>> 0);
  for (let i = copy.length - 1; i > 0; i--) {
    const j = Math.floor(rand() * (i + 1));
    [copy[i], copy[j]] = [copy[j], copy[i]];
  }
  state.order = copy;
  renderList();
}

function seq() {
  const k = "seq:" + state.roundId;
  const n = Number(sessionStorage.getItem(k) || "0") + 1;
  sessionStorage.setItem(k, String(n));
  return n;
}

let voteGen = 0;

async function choose(w) {
  if (note.dataset.kind === "error") setNote("");
  if (w.id === "pending") return;
  if (w.id === state.vote) {
    await castVote(null);
    return;
  }
  await castVote(w.id);
}

async function castVote(wordId) {
  const gen = ++voteGen;
  const prev = state.vote;
  state.vote = wordId;
  renderList();
  try {
    const body = await api("/v1/rounds/" + state.roundId + "/vote", {
      method: "PUT",
      body: JSON.stringify({ word_id: wordId, seq: seq() }),
    });
    if (gen !== voteGen) return;
    if (body.code === "STALE_SEQ") {
      state.vote = prev;
      renderList();
      setNote("That vote arrived too late.", "error");
    }
  } catch (err) {
    if (gen !== voteGen) return;
    state.vote = prev;
    renderList();
    setNote(err.message, "error");
  }
}

document.querySelector("#compose").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (note.dataset.kind === "error") setNote("");
  const text = entry.value;
  if (!text.trim() || state.phase !== "submit") return;
  const gen = ++voteGen;
  const prevVote = state.vote;
  const optimistic = { id: "pending", text: text.trim() };
  place(optimistic);
  state.vote = "pending";
  renderList();
  try {
    const body = await api("/v1/rounds/" + state.roundId + "/words", {
      method: "POST",
      body: JSON.stringify({ text, seq: seq() }),
    });
    state.order = state.order.filter((w) => w.id !== "pending");
    state.mine = body.word_id;
    if (gen === voteGen) state.vote = body.word_id;
    place({ id: body.word_id, text: body.text });
    entry.value = "";
    renderList();
  } catch (err) {
    state.order = state.order.filter((w) => w.id !== "pending");
    if (gen === voteGen) state.vote = prevVote;
    renderList();
    setNote(err.message, "error");
  }
});

search.addEventListener("input", renderList);
document.querySelector("#find").addEventListener("click", searchNow);
pagesEl.addEventListener("click", (e) => {
  if (e.target.closest(".type-slot")) openDialog();
});
const howBtn = document.querySelector("#how");
const rulesEl = document.querySelector("#rules");
howBtn.addEventListener("click", () => {
  const open = rulesEl.hidden;
  rulesEl.hidden = !open;
  howBtn.setAttribute("aria-expanded", open ? "true" : "false");
});
document.querySelector("#close").addEventListener("click", () => document.querySelector("#box").close());
pastBtn.addEventListener("click", () => {
  if (document.body.classList.contains("shelf")) {
    backToToday();
    return;
  }
  showShelf();
  if (location.hash !== "#past") history.pushState({ shelf: true }, "", "#past");
});
document.querySelector(".brand").addEventListener("click", backToToday);
filesEl.addEventListener("click", (e) => {
  const file = e.target.closest(".file");
  if (!file) return;
  state.view = file.dataset.day;
  if (location.hash) history.pushState({ shelf: false }, "", location.pathname);
  showSheet();
  loadEssay(state.view);
});
window.addEventListener("popstate", () => {
  if (location.hash === "#past") showShelf();
  else showSheet();
});

function onEvent(name, raw) {
  const data = JSON.parse(raw);
  if (name === "round.phase") {
    const changed = applySnap(data);
    if (changed) loadWords();
    return;
  }
  if (name === "word.approved" && data.round_id === state.roundId) {
    place({ id: data.word_id, text: data.text });
    renderList();
  }
  if (name === "round.winner") {
    const cached = state.essays.get(data.day);
    if (cached) cached.push(data.text);
    if (state.view === data.day && !document.body.classList.contains("shelf")) {
      state.fragments.push(data.text);
      renderEssay();
    }
  }
  if (name === "round.skipped" || name === "round.winner") {
    if (data.round_id === state.roundId) {
      state.words = [];
      state.order = [];
      state.mine = null;
      state.vote = null;
      renderList();
    }
  }
}

async function boot() {
  try {
    await restoreSession();
  } catch (err) {
    openedEl.hidden = false;
    openedEl.textContent = err.message || "Could not start a session.";
    return;
  }
  const cur = await api("/v1/rounds/current");
  applySnap(cur);
  await loadEssay(state.today);
  await loadWords();
  if (location.hash === "#past") showShelf();
  const es = new EventSource("/v1/events");
  for (const name of ["word.approved", "round.phase", "round.winner", "round.skipped"]) {
    es.addEventListener(name, (ev) => onEvent(name, ev.data));
  }
  setInterval(renderClock, 200);
  renderClock();
}

boot().catch((err) => { setNote(err.message, "error"); });
