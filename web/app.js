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

function serverNow() { return Date.now() + state.offset; }

function utcDay(ms) { return new Date(ms).toISOString().slice(0, 10); }

async function api(path, opt = {}) {
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

async function choose(w) {
  if (note.dataset.kind === "error") setNote("");
  const prev = state.vote;
  state.vote = w.id;
  renderList();
  try {
    const body = await api("/v1/rounds/" + state.roundId + "/vote", {
      method: "PUT",
      body: JSON.stringify({ word_id: w.id, seq: seq() }),
    });
    if (body.code === "STALE_SEQ") setNote("That vote arrived too late.", "error");
  } catch (err) {
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
  const optimistic = { id: "pending", text: text.trim() };
  place(optimistic);
  renderList();
  try {
    const body = await api("/v1/rounds/" + state.roundId + "/words", {
      method: "POST",
      body: JSON.stringify({ text, seq: seq() }),
    });
    state.order = state.order.filter((w) => w.id !== "pending");
    state.mine = body.word_id;
    place({ id: body.word_id, text: body.text });
    entry.value = "";
    renderList();
  } catch (err) {
    state.order = state.order.filter((w) => w.id !== "pending");
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
  await api("/v1/sessions", { method: "POST", body: "{}" });
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
