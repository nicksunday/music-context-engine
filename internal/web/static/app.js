const state = {
  context: null,
  currentBatch: null,
  sessions: [],
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
};

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

async function loadRail() {
  renderRows(elements.releaseList, [], () => ({}), "Loading releases");
  const rail = await api("/api/rail");
  renderRail(rail);
}

async function loadLatestBatch() {
  const response = await api("/api/batch/latest");
  setCurrentBatch(response.batch);
}

async function loadSessions() {
  const response = await api("/api/batches");
  state.sessions = response.batches || [];
  renderSessions(state.sessions);
}

async function loadSession(session) {
  setStatus("Loading session");
  try {
    const response = await api(`/api/batch?id=${encodeURIComponent(session.id)}`);
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
    meta.textContent = [formatSessionDate(session.created_at), `${session.candidate_count || 0} albums`].join(" · ");
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
  if (!batch || !batch.candidates || batch.candidates.length === 0) {
    elements.batchMeta.textContent = "No current batch";
    elements.candidateList.append(emptyNode("No current batch"));
    return;
  }
  const count = batch.candidates.length;
  elements.batchMeta.textContent = `${count} album${count === 1 ? "" : "s"} from the latest verified discovery run`;
  for (const candidate of batch.candidates) {
    const card = document.createElement("article");
    card.className = "candidate";
    card.dataset.candidateId = candidate.id;

    const title = document.createElement("h3");
    if (candidate.streaming_url) {
      const link = document.createElement("a");
      link.href = candidate.streaming_url;
      link.target = "_blank";
      link.rel = "noopener";
      link.textContent = candidate.album;
      title.append(link);
    } else {
      title.textContent = candidate.album;
    }
    const artist = document.createElement("div");
    artist.className = "meta";
    artist.textContent = candidate.artist;
    const starter = document.createElement("div");
    starter.className = "matched-track";
    const starterLabel = document.createElement("span");
    starterLabel.textContent = "Matched track";
    const starterValue = document.createElement("strong");
    starterValue.textContent = candidate.starter_track || "No matched track";
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

async function refreshAll() {
  await Promise.all([loadContext(), loadRail(), loadLatestBatch(), loadSessions()]);
}

elements.promptForm.addEventListener("submit", submitPrompt);
elements.quickFeedback.addEventListener("submit", submitQuickFeedback);
elements.refreshContext.addEventListener("click", () => refreshAll().catch((error) => appendMessage("error", error.message)));

renderBatch(null);
renderSessions([]);
refreshAll().catch((error) => appendMessage("error", error.message));
