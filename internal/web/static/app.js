const state = {
  context: null,
  currentBatch: null,
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
  thread: document.querySelector("#thread"),
  batchMeta: document.querySelector("#batchMeta"),
  candidateList: document.querySelector("#candidateList"),
  promptForm: document.querySelector("#promptForm"),
  quickFeedback: document.querySelector("#quickFeedback"),
  refreshContext: document.querySelector("#refreshContext"),
  sendButton: document.querySelector("#sendButton"),
  statusLine: document.querySelector("#statusLine"),
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
    state.currentBatch = response.batch;
    appendMessage("assistant", response.reply || "Batch generated.");
    renderBatch(response.batch);
    await loadContext();
    await loadRail();
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
    title.textContent = candidate.album;
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

async function refreshAll() {
  await loadContext();
  await loadRail();
}

elements.promptForm.addEventListener("submit", submitPrompt);
elements.quickFeedback.addEventListener("submit", submitQuickFeedback);
elements.refreshContext.addEventListener("click", () => refreshAll().catch((error) => appendMessage("error", error.message)));

renderBatch(null);
refreshAll().catch((error) => appendMessage("error", error.message));
