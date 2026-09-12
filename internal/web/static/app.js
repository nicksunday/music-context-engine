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
  limit: document.querySelector("#limit"),
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
  exportReview: document.querySelector("#exportReview"),
  exportPreview: document.querySelector("#exportPreview"),
  approveExport: document.querySelector("#approveExport"),
  downloadExport: document.querySelector("#downloadExport"),
  exampleList: document.querySelector("#exampleList"),
  exampleForm: document.querySelector("#exampleForm"),
  loadExamples: document.querySelector("#loadExamples"),
};

const exportState = { items: [], selected: new Set(), payloads: new Map() };
state.examples = [];

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

async function loadExportReview() {
  const response = await api("/api/export/review");
  exportState.items = response.items || [];
  elements.exportReview.replaceChildren();
  for (const item of exportState.items) {
    const label = document.createElement("label");
    label.className = "export-item";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = exportState.selected.has(item.feedback_id);
    checkbox.addEventListener("change", () => {
      if (checkbox.checked) exportState.selected.add(item.feedback_id); else exportState.selected.delete(item.feedback_id);
      updateExportPreview();
    });
    const text = document.createElement("span");
    text.textContent = `${item.mode}: ${item.request} — ${item.verdict}${item.approved ? " (approved)" : ""}`;
    label.append(checkbox, text);
    elements.exportReview.append(label);
  }
  updateExportPreview();
}

async function updateExportPreview() {
  const examples = [];
  for (const item of exportState.items.filter(value => exportState.selected.has(value.feedback_id))) {
    try {
      const example = await api(`/api/export/example?feedback_id=${encodeURIComponent(item.feedback_id)}`);
      examples.push(example);
      exportState.payloads.set(item.feedback_id, example);
    } catch (error) { appendMessage("error", error.message); }
  }
  elements.exportPreview.hidden = examples.length === 0;
  elements.exportPreview.value = examples.map(example => JSON.stringify(example)).join("\n");
  elements.approveExport.disabled = examples.length === 0;
  elements.downloadExport.disabled = examples.length === 0;
}

async function approveSelectedExports() {
  const editedLines = elements.exportPreview.value.split("\n").map(line => line.trim()).filter(Boolean);
  const selectedItems = exportState.items.filter(value => exportState.selected.has(value.feedback_id));
  if (editedLines.length !== selectedItems.length) throw new Error("Export preview must contain one JSON object per selected example.");
  for (const [index, item] of selectedItems.entries()) {
    let payload;
    try { payload = JSON.parse(editedLines[index]); } catch (_) { throw new Error("Export preview contains invalid JSON."); }
    if (!payload) continue;
    const draft = await api("/api/export/draft", { method: "POST", body: JSON.stringify({ feedback_id: item.feedback_id, payload }) });
    await api("/api/export/approve", { method: "POST", body: JSON.stringify({ draft_id: draft.id }) });
  }
  await loadExportReview();
}

async function downloadSelectedExports() {
  const draftIDs = exportState.items.filter(item => exportState.selected.has(item.feedback_id) && item.draft_id && item.approved).map(item => item.draft_id);
  if (!draftIDs.length) throw new Error("Approve selected examples before downloading.");
  const response = await fetch("/api/export/download", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ draft_ids: draftIDs }) });
  if (!response.ok) throw new Error("Export download failed");
  const blob = await response.blob();
  const link = document.createElement("a"); link.href = URL.createObjectURL(blob); link.download = "music-vault-fit-examples.jsonl"; link.click(); URL.revokeObjectURL(link.href);
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
    restorePrompt(response.batch || session);
    await loadExamples();
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
  renderRows(elements.feedbackList, (context.recent_feedback || []).slice(0, 8), (feedback) => ({
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
    examples: state.examples || [],
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
    if (response.batch) {
      response.batch.degraded_reason = response.degraded_reason || "";
      response.batch.shortfall_reason = response.shortfall_reason || "";
    }
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

function exampleRequestFields() {
  return {
    message: elements.message.value.trim(),
    mood: elements.mood.value.trim(),
    avoid: document.querySelector("#avoid").value.trim(),
    mode: state.mode === "song" ? "song" : "album",
  };
}

async function loadExamples() {
  const fields = exampleRequestFields();
  if (!fields.message) return;
  const query = new URLSearchParams(fields);
  const response = await api(`/api/examples?${query}`);
  state.examples = response.examples || [];
  renderExamples();
}

function renderExamples() {
  elements.exampleList.replaceChildren();
  for (const example of state.examples) {
    const row = document.createElement("div");
    row.className = "example-row";
    row.textContent = `${example.entity_scope}: ${example.supplied_text} · ${example.polarity}${example.notes ? ` · ${example.notes}` : ""}`;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "button secondary";
    remove.textContent = "Remove";
    remove.addEventListener("click", async () => {
      await api("/api/examples", { method: "DELETE", body: JSON.stringify({ ...exampleRequestFields(), example }) });
      await loadExamples();
    });
    row.append(remove);
    elements.exampleList.append(row);
  }
}

async function saveExample(event) {
  event.preventDefault();
  const form = new FormData(elements.exampleForm);
  const example = Object.fromEntries(form.entries());
  if (example.entity_scope === "artist") example.song = "";
  await api("/api/examples", { method: "POST", body: JSON.stringify({ ...exampleRequestFields(), example }) });
  await loadExamples();
  elements.exampleForm.reset();
  updateExampleSongVisibility();
}

function updateExampleSongVisibility() {
  const scope = elements.exampleForm.elements.entity_scope.value;
  const field = elements.exampleForm.querySelector(".example-song-field");
  field.hidden = scope === "artist";
  field.querySelector("input").disabled = scope === "artist";
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
  const state = [batch.degraded_reason && `degraded: ${batch.degraded_reason}`, batch.shortfall_reason && `shortfall: ${batch.shortfall_reason}`].filter(Boolean).join(" · ");
  elements.batchMeta.textContent = `${count} ${isSongBatch ? "song" : "album"}${count === 1 ? "" : "s"} from the latest verified discovery run${state ? ` · ${state}` : ""}`;
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
    card.append(renderPromptFitControls(candidate));
    elements.candidateList.append(card);
  }
}

function renderPromptFitControls(candidate) {
  const group = document.createElement("div");
  group.className = "prompt-fit";
  const label = document.createElement("div");
  label.className = "prompt-fit-label";
  label.textContent = "Request fit";
  const actions = document.createElement("div");
  actions.className = "prompt-fit-actions";
  for (const [verdict, text] of [["met", "Met my request"], ["missed", "Missed my request"]]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "button secondary prompt-fit-button";
    button.textContent = text;
    if (candidate.prompt_fit?.verdict === verdict) button.classList.add("selected");
    button.addEventListener("click", () => savePromptFit(candidate, verdict, group));
    actions.append(button);
  }
  if (candidate.prompt_fit) {
    const clear = document.createElement("button");
    clear.type = "button";
    clear.className = "button secondary";
    clear.textContent = "Clear";
    clear.addEventListener("click", () => clearPromptFit(candidate));
    actions.append(clear);
  }
  const reason = document.createElement("select");
  reason.className = "prompt-fit-reason";
  reason.innerHTML = '<option value="">Reason (optional)</option><option value="wrong_genre">Wrong genre</option><option value="wrong_energy">Wrong energy</option><option value="violated_exclusion">Violated exclusion</option><option value="inaccurate_explanation">Inaccurate explanation</option><option value="other">Other</option>';
  reason.value = candidate.prompt_fit?.reason || "";
  const notes = document.createElement("input");
  notes.className = "prompt-fit-notes";
  notes.placeholder = "Optional note";
  notes.value = candidate.prompt_fit?.notes || "";
  group.append(label, actions, reason, notes);
  return group;
}

async function savePromptFit(candidate, verdict, group) {
  const buttons = group.querySelectorAll("button");
  buttons.forEach(button => button.disabled = true);
  try {
    const response = await api("/api/prompt-fit", { method: "POST", body: JSON.stringify({
      batch_id: candidate.batch_id,
      candidate_id: candidate.id,
      verdict,
      reason: group.querySelector(".prompt-fit-reason").value,
      notes: group.querySelector(".prompt-fit-notes").value.trim(),
    }) });
    candidate.prompt_fit = response;
    renderBatch(state.currentBatch);
    setStatus("Request-fit judgment saved");
  } catch (error) {
    appendMessage("error", error.message);
    buttons.forEach(button => button.disabled = false);
  }
}

async function clearPromptFit(candidate) {
  try {
    await api("/api/prompt-fit", { method: "DELETE", body: JSON.stringify({ batch_id: candidate.batch_id, candidate_id: candidate.id }) });
    delete candidate.prompt_fit;
    renderBatch(state.currentBatch);
    setStatus("Request-fit judgment cleared");
  } catch (error) {
    appendMessage("error", error.message);
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
  row.append(identity, actions, renderPromptFitControls(candidate));
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
  elements.limit.max = state.mode === "song" ? "20" : "10";
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
  await Promise.all([loadContext(), loadRail(), loadLatestBatch(), loadSessions(), loadExportReview()]);
}

elements.promptForm.addEventListener("submit", submitPrompt);
elements.exampleForm.addEventListener("submit", saveExample);
elements.loadExamples.addEventListener("click", () => loadExamples().catch((error) => appendMessage("error", error.message)));
elements.exampleForm.elements.entity_scope.addEventListener("change", updateExampleSongVisibility);
updateExampleSongVisibility();
elements.quickFeedback.addEventListener("submit", submitQuickFeedback);
elements.refreshContext.addEventListener("click", () => refreshAll().catch((error) => appendMessage("error", error.message)));
elements.approveExport.addEventListener("click", () => approveSelectedExports().catch((error) => appendMessage("error", error.message)));
elements.downloadExport.addEventListener("click", () => downloadSelectedExports().catch((error) => appendMessage("error", error.message)));

configureMode();
renderBatch(null);
renderSessions([]);
refreshAll().catch((error) => appendMessage("error", error.message));
