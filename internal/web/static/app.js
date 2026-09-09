const state = {
  context: null,
  currentBatch: null,
  sessions: [],
  mode: (() => {
    const path = window.location.pathname.replace(/\/+$/, "") || "/";
    if (path === "/recommendations/songs") return "song";
    if (path === "/recommendations/albums") return "album";
    return "entry";
  })(),
};

const defaultPrompts = {
  song: "Give me a handful of individual songs to hear next.",
  album: "Give me a handful of high-impact albums to check out next.",
};

const verdictLabels = {
  disliked: "Disliked",
  not_for_me_today: "Not Today",
  ok: "OK",
  good: "Good",
  great: "Great",
  already_know: "Know It",
};

const elements = {
  modelLine: document.querySelector("#modelLine"),
  releaseList: document.querySelector("#releaseList"),
  feedbackList: document.querySelector("#feedbackList"),
  sessionList: document.querySelector("#sessionList"),
  thread: document.querySelector("#thread"),
  batchMeta: document.querySelector("#batchMeta"),
  candidateList: document.querySelector("#candidateList"),
  promptForm: document.querySelector("#promptForm"),
  quickFeedback: document.querySelector("#quickFeedback"),
  refreshContext: document.querySelector("#refreshContext"),
  sendButton: document.querySelector("#sendButton"),
  statusLine: document.querySelector("#statusLine"),
  message: document.querySelector("#message"),
  mood: document.querySelector("#mood"),
  pageTitle: document.querySelector("#pageTitle"),
  pageDescription: document.querySelector("#pageDescription"),
  sessionHeading: document.querySelector("#sessionHeading"),
  modeChooser: document.querySelector("#modeChooser"),
};

const initialPrompt = elements.message.value;

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || `Request failed with ${response.status}`);
  }
  return payload;
}

async function loadContext() {
  setStatus("Loading context");
  const context = await api("/api/context");
  state.context = context;
  renderContext(context);
  setStatus("");
}

function openAppleMusic(url, appURL) {
  if (!url) return;
  const isMacDesktop = /Macintosh|Mac OS X/i.test(navigator.userAgent) && !/Mobile|iPhone|iPad/i.test(navigator.userAgent);
  if (!isMacDesktop || !appURL) {
    window.open(url, "_blank", "noopener");
    return;
  }
  const fallback = window.setTimeout(() => window.open(url, "_blank", "noopener"), 900);
  const cancelFallback = () => window.clearTimeout(fallback);
  window.addEventListener("blur", cancelFallback, { once: true });
  const frame = document.createElement("iframe");
  frame.hidden = true;
  frame.src = appURL;
  document.body.append(frame);
  window.setTimeout(() => frame.remove(), 1500);
}

async function loadRail() {
  renderRows(elements.releaseList, [], () => ({}), "Loading releases");
  const rail = await api("/api/rail");
  renderRail(rail);
}

async function loadLatestBatch() {
  if (state.mode === "entry") return setCurrentBatch(null);
  const response = await api(`/api/batch/latest?mode=${encodeURIComponent(state.mode)}`);
  setCurrentBatch(response.batch);
}

async function loadSessions() {
  if (state.mode === "entry") {
    state.sessions = [];
    return renderSessions([]);
  }
  const response = await api(`/api/batches?mode=${encodeURIComponent(state.mode)}`);
  state.sessions = response.batches || [];
  renderSessions(state.sessions);
}

async function loadSession(session) {
  setStatus("Loading session");
  try {
    const response = await api(`/api/batch?id=${encodeURIComponent(session.id)}&mode=${encodeURIComponent(state.mode)}`);
    setCurrentBatch(response.batch);
  } catch (error) {
    // Partial recovery (D3): restore the prompt client-side and surface an
    // explicit error in the Current Batch surface instead of a silent no-op.
    restorePrompt(session);
    renderBatchLoadError(`Could not load this session (${error.message}). Its prompt was restored below and is still editable.`);
  } finally {
    setStatus("");
  }
}

function setCurrentBatch(batch) {
  state.currentBatch = batch;
  renderBatch(batch);
}

// Client-side partial recovery (D2/D3): repopulate the prompt composer with a
// session's stored prompt/mood and focus it. No network call, no batch
// generation — purely editing the input for later (re)submission.
function restorePrompt(session) {
  if (!session) return;
  if (session.prompt != null) {
    elements.message.value = session.prompt;
    if (session.mood) elements.mood.value = session.mood;
    elements.message.focus();
    elements.message.scrollTop = elements.message.scrollHeight;
    setStatus("Prompt restored — edit or submit with Generate Batch when ready.");
  }
}

// Render an explicit, non-empty failure state in the Current Batch surface so a
// failed load is never a silent no-op (D3 / "Browse previous session prompts").
function renderBatchLoadError(message) {
  state.currentBatch = null;
  elements.candidateList.replaceChildren();
  elements.batchMeta.textContent = "Could not load session";
  const node = document.createElement("div");
  node.className = "empty session-load-failed";
  node.textContent = message;
  elements.candidateList.append(node);
}

function renderSessions(sessions) {
  elements.sessionList.replaceChildren();
  if (!sessions || sessions.length === 0) {
    elements.sessionList.append(emptyNode("No previous sessions"));
    return;
  }

  for (const session of sessions) {
    const row = document.createElement("div");
    row.className = "session-row";
    row.tabIndex = 0;
    row.setAttribute("role", "group");

    const prompt = document.createElement("strong");
    prompt.textContent = session.prompt || "Untitled prompt";
    const meta = document.createElement("span");
    meta.textContent = [formatSessionDate(session.created_at), `${session.candidate_count || 0} ${session.mode === "song" ? "songs" : "albums"}`].join(" · ");
    const reply = document.createElement("span");
    reply.className = "session-reply";
    reply.textContent = session.reply || "";

    const controls = document.createElement("div");
    controls.className = "session-actions";

    const loadButton = document.createElement("button");
    loadButton.type = "button";
    loadButton.className = "session-load";
    loadButton.textContent = "Load";
    loadButton.addEventListener("click", () => loadSession(session));

    const restoreButton = document.createElement("button");
    restoreButton.type = "button";
    restoreButton.className = "session-restore";
    restoreButton.textContent = "Restore prompt";
    restoreButton.addEventListener("click", () => restorePrompt(session));

    controls.append(loadButton, restoreButton);

    // Tab/enter parity with the old row button (D1 risk note): the row is
    // focusable as a group and Enter triggers the load action.
    row.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        loadSession(session);
      }
    });

    row.append(prompt, meta);
    if (reply.textContent) row.append(reply);
    row.append(controls);
    elements.sessionList.append(row);
  }
}

function renderContext(context) {
  elements.modelLine.textContent = `${context.model || "no model"} at ${context.ollama_url || "no Ollama URL"}`;
  renderRows(elements.feedbackList, context.recent_feedback.slice(0, 8), (feedback) => ({
    name: `${feedback.album} · ${feedback.artist}`,
    meta: `${feedback.verdict}${feedback.notes ? ` · ${feedback.notes}` : ""}`,
  }));
}

function renderRail(rail) {
  renderRows(elements.releaseList, rail.new_releases, (release) => ({
    name: `${release.album} · ${release.artist}`,
    meta: [formatReleaseDate(release.release_date), release.release_type].filter(Boolean).join(" · "),
    detail: release.blurb || "",
  }), rail.release_error || "No new releases found");
}

function renderRows(container, values, mapValue, emptyText = "No rows") {
  container.replaceChildren();
  if (!values || values.length === 0) {
    container.append(emptyNode(emptyText));
    return;
  }
  for (const value of values) {
    const mapped = mapValue(value);
    const row = document.createElement("div");
    row.className = "row";
    const name = document.createElement("div");
    name.className = "name";
    name.textContent = mapped.name;
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = mapped.meta;
    row.append(name, meta);
    if (mapped.detail) {
      const detail = document.createElement("div");
      detail.className = "detail";
      detail.textContent = mapped.detail;
      row.append(detail);
    }
    container.append(row);
  }
}

function emptyNode(text) {
  const node = document.createElement("div");
  node.className = "empty";
  node.textContent = text;
  return node;
}

function appendMessage(kind, text) {
  const message = document.createElement("div");
  message.className = `message ${kind}`;
  message.textContent = text;
  elements.thread.append(message);
  elements.thread.scrollTop = elements.thread.scrollHeight;
}

async function submitPrompt(event) {
  event.preventDefault();
  const form = new FormData(elements.promptForm);
  const request = {
    message: String(form.get("message") || "").trim(),
    mood: String(form.get("mood") || "").trim(),
    avoid: String(form.get("avoid") || "").trim(),
    limit: Number(form.get("limit") || 6),
    mode: state.mode === "song" ? "song" : "album",
  };
  if (!request.message) return;

  appendMessage("user", request.message);
  elements.sendButton.disabled = true;
  setStatus("Generating");
  try {
    const response = await api("/api/recommendations", {
      method: "POST",
      body: JSON.stringify(request),
    });
    setCurrentBatch(response.batch);
    appendMessage("assistant", response.reply || "Batch generated.");
    await loadContext();
    await loadRail();
    await loadSessions();
  } catch (error) {
    appendMessage("error", error.message);
  } finally {
    elements.sendButton.disabled = false;
    setStatus("");
  }
}

function renderBatch(batch) {
  elements.candidateList.replaceChildren();
  elements.candidateList.classList.toggle("song-list", batch?.mode === "song");
  if (!batch || !batch.candidates || batch.candidates.length === 0) {
    elements.batchMeta.textContent = "No current batch";
    elements.candidateList.append(emptyNode("No current batch"));
    return;
  }
  const count = batch.candidates.length;
  const isSongBatch = batch.mode === "song";
  elements.batchMeta.textContent = `${count} ${isSongBatch ? "song" : "album"}${count === 1 ? "" : "s"} from the latest verified discovery run`;
  for (const candidate of batch.candidates) {
    if (isSongBatch) {
      renderSongCandidate(candidate);
      continue;
    }
    const card = document.createElement("article");
    card.className = "candidate";
    card.dataset.candidateId = candidate.id;

    const title = document.createElement("h3");
    if (candidate.streaming_url) {
      if (isSongBatch) {
        const appLink = document.createElement("button");
        appLink.type = "button";
        appLink.className = "app-link";
        appLink.textContent = candidate.song || candidate.starter_track || "Open in Apple Music";
        appLink.title = "Open in Apple Music";
        appLink.addEventListener("click", () => openAppleMusic(candidate.streaming_url, candidate.streaming_app_url));
        title.append(appLink);
      } else {
        const link = document.createElement("a");
        link.href = candidate.streaming_url;
        link.target = "_blank";
        link.rel = "noopener";
        link.textContent = candidate.album;
        title.append(link);
      }
    } else {
      title.textContent = isSongBatch ? (candidate.song || candidate.starter_track || "Untitled song") : candidate.album;
    }
    const artist = document.createElement("div");
    artist.className = "meta";
    artist.textContent = candidate.artist;
    const starter = document.createElement("div");
    starter.className = "matched-track";
    const starterLabel = document.createElement("span");
    starterLabel.textContent = "Matched track";
    const starterValue = document.createElement("strong");
    starterValue.textContent = isSongBatch ? (candidate.album || "No album context") : (candidate.starter_track || "No matched track");
    starter.append(starterLabel, starterValue);
    const note = document.createElement("div");
    note.className = "note";
    note.textContent = candidate.note || "";

    const tags = document.createElement("div");
    tags.className = "tags";
    for (const tag of candidate.genre_tags || []) {
      const tagNode = document.createElement("span");
      tagNode.className = "tag";
      tagNode.textContent = tag;
      tags.append(tagNode);
    }

    const buttons = document.createElement("div");
    buttons.className = "verdicts";
    for (const verdict of ["disliked", "not_for_me_today", "ok", "good", "great", "already_know"]) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = verdictLabels[verdict];
      button.addEventListener("click", () => logCandidateFeedback(candidate, verdict, button));
      buttons.append(button);
    }

    card.append(title, artist, starter);
    if (note.textContent) card.append(note);
    if (tags.children.length > 0) card.append(tags);
    card.append(buttons);
    elements.candidateList.append(card);
  }
}

function renderSongCandidate(candidate) {
  const row = document.createElement("article");
  row.className = "song-row";
  row.dataset.candidateId = candidate.id;
  const identity = document.createElement("div");
  identity.className = "song-identity";
  const title = document.createElement("h3");
  title.textContent = candidate.song || candidate.starter_track || "Untitled song";
  const artist = document.createElement("div");
  artist.className = "meta";
  artist.textContent = [candidate.artist, candidate.album].filter(Boolean).join(" · ");
  identity.append(title, artist);
  const actions = document.createElement("div");
  actions.className = "song-actions";
  if (candidate.streaming_url) {
    const link = document.createElement("a");
    link.href = candidate.streaming_url;
    link.target = "_blank";
    link.rel = "noopener";
    link.textContent = "Open song";
    actions.append(link);
  }
  for (const [verdict, label] of [["good", "Liked"], ["disliked", "Disliked"]]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "button secondary song-feedback";
    button.textContent = label;
    button.addEventListener("click", () => logCandidateFeedback(candidate, verdict, button));
    actions.append(button);
  }
  const dismiss = document.createElement("button");
  dismiss.type = "button";
  dismiss.className = "button secondary song-feedback";
  dismiss.textContent = "Not Today";
  dismiss.addEventListener("click", () => {
    row.remove();
    state.currentBatch.candidates = state.currentBatch.candidates.filter(item => item.id !== candidate.id);
    setStatus("Removed for now");
  });
  actions.append(dismiss);
  row.append(identity, actions);
  elements.candidateList.append(row);
}

async function logCandidateFeedback(candidate, verdict, button) {
  button.disabled = true;
  setStatus("Logging feedback");
  try {
    await api("/api/feedback", {
      method: "POST",
      body: JSON.stringify({
        artist: candidate.artist,
        album: candidate.album,
        starter_track: candidate.starter_track,
        batch_id: candidate.batch_id,
        candidate_id: candidate.id,
        verdict,
        notes: candidate.note || "",
      }),
    });
    for (const sibling of button.parentElement.querySelectorAll("button")) {
      sibling.classList.remove("selected");
    }
    button.classList.add("selected");
    await loadContext();
    await loadRail();
  } catch (error) {
    appendMessage("error", error.message);
  } finally {
    button.disabled = false;
    setStatus("");
  }
}

async function submitQuickFeedback(event) {
  event.preventDefault();
  const form = new FormData(elements.quickFeedback);
  const payload = {
    artist: String(form.get("artist") || "").trim(),
    album: String(form.get("album") || "").trim(),
    verdict: String(form.get("verdict") || "").trim(),
    notes: String(form.get("notes") || "").trim(),
  };
  setStatus("Logging feedback");
  try {
    await api("/api/feedback", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    elements.quickFeedback.reset();
    await loadContext();
    await loadRail();
  } catch (error) {
    appendMessage("error", error.message);
  } finally {
    setStatus("");
  }
}

function setStatus(text) {
  elements.statusLine.textContent = text;
}

function formatReleaseDate(value) {
  if (!value) return "";
  return value;
}

function formatSessionDate(value) {
  if (!value) return "Unknown date";
  const date = new Date(value.replace(" ", "T") + (value.includes("Z") ? "" : "Z"));
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function configureMode() {
  const labels = {
    entry: ["Recommendation Engine", "Choose a recommendation workflow or generate a verified batch."],
    song: ["Song Recommendations", "Discover individual tracks in a compact list with quick feedback."],
    album: ["Album Recommendations", "Explore verified albums with richer context and album-level feedback."],
  };
  const [title, description] = labels[state.mode];
  elements.pageTitle.textContent = title;
  elements.pageDescription.textContent = description;
  elements.sessionHeading.textContent = state.mode === "song" ? "Song Sessions" : "Album Sessions";
  if (defaultPrompts[state.mode] && (!elements.message.value || elements.message.value === initialPrompt)) {
    elements.message.value = defaultPrompts[state.mode];
  }
  elements.modeChooser.hidden = state.mode !== "entry";
  document.querySelectorAll(".recommendation-nav a").forEach(link => {
    link.classList.toggle("active", link.dataset.mode === state.mode);
  });
  elements.promptForm.hidden = state.mode === "entry";
}

async function refreshAll() {
  await Promise.all([loadContext(), loadRail(), loadLatestBatch(), loadSessions()]);
}

elements.promptForm.addEventListener("submit", submitPrompt);
elements.quickFeedback.addEventListener("submit", submitQuickFeedback);
elements.refreshContext.addEventListener("click", () => refreshAll().catch((error) => appendMessage("error", error.message)));

configureMode();
renderBatch(null);
renderSessions([]);
refreshAll().catch((error) => appendMessage("error", error.message));
