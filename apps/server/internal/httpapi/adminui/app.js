const state = {
  instance: null,
  session: readSession(),
  books: [],
  readers: [],
  me: null,
  selectedBook: null,
};

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

document.addEventListener("DOMContentLoaded", start);

async function start() {
  bindEvents();
  try {
    state.instance = await request("/api/v1/instance", { auth: false });
    $("#instance-name").textContent = state.instance.name;
    document.title = `${state.instance.name} administration`;
    if (state.instance.setupRequired) {
      showBootstrap();
      return;
    }
    if (state.session) {
      try {
        const user = state.me = await request("/api/v1/me");
        if (user.role !== "admin") throw new APIError(403, "forbidden", "Administrator access is required.");
        await openConsole();
        return;
      } catch (error) {
        clearSession();
        if (error.status === 403) setError("login", error.message);
      }
    }
    showLogin();
  } catch (error) {
    showLogin();
    setError("login", readableError(error, "This BookHarbor server could not be reached."));
  }
}

function bindEvents() {
  $("#login-form").addEventListener("submit", login);
  $("#bootstrap-form").addEventListener("submit", bootstrap);
  $("#sign-out").addEventListener("click", signOut);
  $$(".nav-item").forEach((button) => button.addEventListener("click", () => changeView(button.dataset.view)));
  $("#toggle-upload").addEventListener("click", () => $("#upload-form").hidden = false);
  $("[data-cancel-upload]").addEventListener("click", () => $("#upload-form").hidden = true);
  $("#upload-form").addEventListener("submit", uploadBook);
  $("#metadata-form").addEventListener("submit", saveMetadata);
  $("#hardcover-form").addEventListener("submit", searchHardcover);
  $("#close-editor").addEventListener("click", closeEditor);
  $("#reader-form").addEventListener("submit", createReader);
  $("#refresh-activity").addEventListener("click", loadActivity);
  $$("[data-close-dialog]").forEach((button) => button.addEventListener("click", () => button.closest("dialog").close()));
}

function showLogin() {
  $("#entry").hidden = false;
  $("#console").hidden = true;
  $("#login-form").hidden = false;
  $("#bootstrap-form").hidden = true;
  $("#entry-title").textContent = "Welcome aboard";
  $("#entry-help").textContent = "Sign in with an administrator account.";
}

function showBootstrap() {
  $("#entry").hidden = false;
  $("#console").hidden = true;
  $("#login-form").hidden = true;
  $("#bootstrap-form").hidden = false;
  $("#entry-title").textContent = "Set up your harbor";
  $("#entry-help").textContent = "Create the first administrator for this server.";
}

async function openConsole() {
  $("#entry").hidden = true;
  $("#console").hidden = false;
  $("#account-avatar").textContent = initials(state.me?.displayName);
  await Promise.all([loadBooks(), loadReaders()]);
}

async function login(event) {
  event.preventDefault();
  setError("login", "");
  await withButton(event.submitter, "Signing in...", async () => {
    const data = formObject(event.currentTarget);
    try {
      state.session = await request("/api/v1/sessions", { method: "POST", body: data, auth: false });
      if (state.session.user.role !== "admin") {
        clearSession();
        throw new APIError(403, "forbidden", "This console is available to administrators only.");
      }
      state.me = state.session.user;
      saveSession();
      await openConsole();
    } catch (error) {
      setError("login", readableError(error, "Sign in failed."));
    }
  });
}

async function bootstrap(event) {
  event.preventDefault();
  setError("bootstrap", "");
  await withButton(event.submitter, "Creating...", async () => {
    const data = formObject(event.currentTarget);
    try {
      await request("/api/v1/bootstrap", { method: "POST", body: data, auth: false });
      state.instance.setupRequired = false;
      showLogin();
      $("#login-form [name=email]").value = data.email;
      showToast("Administrator created. Sign in to continue.");
    } catch (error) {
      setError("bootstrap", readableError(error, "Setup could not be completed."));
    }
  });
}

async function signOut() {
  try { await request("/api/v1/sessions/current", { method: "DELETE" }); } catch (_) { /* local sign-out still succeeds */ }
  clearSession();
  showLogin();
}

function changeView(name) {
  $$(".nav-item").forEach((button) => button.classList.toggle("active", button.dataset.view === name));
  $("#library-view").hidden = name !== "library";
  $("#readers-view").hidden = name !== "readers";
  $("#activity-view").hidden = name !== "activity";
  if (name === "activity") loadActivity();
  $("#main").focus();
}

async function loadBooks() {
  $("#library-status").textContent = "Loading library...";
  try {
    const response = await request("/api/v1/books?limit=100");
    state.books = response.items;
    renderBooks();
  } catch (error) {
    $("#library-status").textContent = readableError(error, "The library could not be loaded.");
  }
}

function renderBooks() {
  const list = $("#book-list");
  list.replaceChildren();
  $("#library-status").textContent = state.books.length === 1 ? "1 book" : `${state.books.length} books`;
  if (!state.books.length) {
    list.append(emptyState("No books yet", "Import an EPUB or PDF to begin your library."));
    return;
  }
  state.books.forEach((book) => {
    const button = node("button", "book-row");
    button.type = "button";
    button.classList.toggle("selected", state.selectedBook?.id === book.id);
    button.addEventListener("click", () => openEditor(book));
    const cover = node("span", "book-cover");
    if (book.coverUrl) {
      const image = document.createElement("img");
      image.src = book.coverUrl;
      image.alt = "";
      image.loading = "lazy";
      cover.append(image);
    } else {
      cover.textContent = initials(book.title);
    }
    const details = document.createElement("span");
    details.append(textNode("h3", book.title));
    details.append(textNode("p", book.authors?.join(", ") || book.subtitle || "Metadata not added"));
    details.append(textNode("span", book.editions.map((edition) => edition.format).join(" / "), "format"));
    button.append(cover, details);
    list.append(button);
  });
}

function openEditor(book) {
  state.selectedBook = book;
  const form = $("#metadata-form");
  form.elements.title.value = book.title || "";
  form.elements.subtitle.value = book.subtitle || "";
  form.elements.authors.value = (book.authors || []).join("\n");
  form.elements.description.value = book.description || "";
  form.elements.coverUrl.value = book.coverUrl || "";
  form.elements.sourceProvider.value = book.source?.provider || "";
  form.elements.sourceId.value = book.source?.id || "";
  updateSourceNote();
  $("#hardcover-query").value = book.title;
  $("#hardcover-results").replaceChildren();
  setError("metadata", "");
  setError("hardcover", "");
  $("#book-editor").hidden = false;
  renderBooks();
  if (window.innerWidth < 981) $("#book-editor").scrollIntoView({ behavior: "smooth", block: "start" });
}

function closeEditor() {
  state.selectedBook = null;
  $("#book-editor").hidden = true;
  renderBooks();
}

async function uploadBook(event) {
  event.preventDefault();
  setError("upload", "");
  await withButton(event.submitter, "Importing...", async () => {
    const data = new FormData(event.currentTarget);
    if (!data.get("title")) data.delete("title");
    try {
      const book = await request("/api/v1/books", { method: "POST", body: data });
      event.currentTarget.reset();
      event.currentTarget.hidden = true;
      state.books.unshift(book);
      renderBooks();
      openEditor(book);
      showToast(`${book.title} was imported.`);
    } catch (error) {
      setError("upload", readableError(error, "The book could not be imported."));
    }
  });
}

async function saveMetadata(event) {
  event.preventDefault();
  if (!state.selectedBook) return;
  setError("metadata", "");
  await withButton(event.submitter, "Saving...", async () => {
    const form = event.currentTarget.elements;
    const authors = form.authors.value.split(/\n|,/).map((value) => value.trim()).filter(Boolean);
    const body = {
      title: form.title.value,
      subtitle: form.subtitle.value,
      description: form.description.value,
      authors,
      coverUrl: form.coverUrl.value,
    };
    if (form.sourceProvider.value && form.sourceId.value) {
      body.source = { provider: form.sourceProvider.value, id: form.sourceId.value };
    }
    try {
      const updated = await request(`/api/v1/books/${encodeURIComponent(state.selectedBook.id)}`, { method: "PATCH", body });
      state.books = state.books.map((book) => book.id === updated.id ? updated : book);
      state.selectedBook = updated;
      renderBooks();
      updateSourceNote();
      showToast("Book metadata saved.");
    } catch (error) {
      setError("metadata", readableError(error, "Metadata could not be saved."));
    }
  });
}

async function searchHardcover(event) {
  event.preventDefault();
  setError("hardcover", "");
  const results = $("#hardcover-results");
  results.replaceChildren(textNode("p", "Searching Hardcover...", "field-help"));
  await withButton(event.submitter, "Searching...", async () => {
    try {
      const query = encodeURIComponent(new FormData(event.currentTarget).get("query"));
      const response = await request(`/api/v1/admin/metadata/search?q=${query}`);
      renderHardcoverResults(response.items);
    } catch (error) {
      results.replaceChildren();
      setError("hardcover", readableError(error, "Hardcover search could not be completed."));
    }
  });
}

function renderHardcoverResults(items) {
  const results = $("#hardcover-results");
  results.replaceChildren();
  if (!items.length) {
    results.append(emptyState("No matches", "Try a shorter title or add the author's name."));
    return;
  }
  items.forEach((candidate) => {
    const row = node("div", "provider-result");
    const image = document.createElement("img");
    image.alt = "";
    if (candidate.coverUrl) image.src = candidate.coverUrl;
    const copy = document.createElement("div");
    copy.append(textNode("strong", candidate.title), textNode("span", candidate.authors.join(", ") || candidate.publishedDate || "Hardcover catalog"));
    const use = textNode("button", "Use", "button quiet");
    use.type = "button";
    use.addEventListener("click", () => applyCandidate(candidate));
    row.append(image, copy, use);
    results.append(row);
  });
}

function applyCandidate(candidate) {
  const form = $("#metadata-form").elements;
  form.title.value = candidate.title || form.title.value;
  form.subtitle.value = candidate.subtitle || "";
  form.authors.value = (candidate.authors || []).join("\n");
  form.description.value = candidate.description || "";
  form.coverUrl.value = candidate.coverUrl || "";
  form.sourceProvider.value = candidate.provider;
  form.sourceId.value = candidate.id;
  updateSourceNote();
  showToast("Hardcover details added to the form. Save to apply them.");
}

function updateSourceNote() {
  const form = $("#metadata-form").elements;
  $("#source-note").textContent = form.sourceProvider.value ? `Matched with ${form.sourceProvider.value}.` : "";
}

async function loadReaders() {
  $("#reader-status").textContent = "Loading readers...";
  try {
    const response = await request("/api/v1/admin/users");
    state.readers = response.items;
    renderReaders();
  } catch (error) {
    $("#reader-status").textContent = readableError(error, "Readers could not be loaded.");
  }
}

function renderReaders() {
  const list = $("#reader-list");
  list.replaceChildren();
  $("#reader-status").textContent = state.readers.length === 1 ? "1 account" : `${state.readers.length} accounts`;
  state.readers.forEach((reader) => {
    const isSelf = reader.id === state.me?.id;
    const row = node("div", "reader-row" + (reader.disabled ? " disabled" : ""));
    const avatar = textNode("span", initials(reader.displayName), "reader-avatar");
    const who = document.createElement("div");
    who.append(textNode("strong", reader.displayName + (isSelf ? " (you)" : "")), textNode("span", reader.email));
    const meta = document.createElement("div");
    meta.className = "reader-meta";
    meta.append(textNode("span", reader.disabled ? `${reader.role} · disabled` : reader.role, "role"));
    const time = document.createElement("time");
    time.dateTime = reader.createdAt;
    time.textContent = "Joined " + new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(new Date(reader.createdAt));
    meta.append(time);
    const actions = node("div", "row-actions");
    const label = (verb) => `${verb} ${reader.displayName}`;
    const add = (text, handler, className = "text-button") => {
      const button = textNode("button", text, className);
      button.type = "button";
      button.setAttribute("aria-label", label(text));
      button.addEventListener("click", handler);
      actions.append(button);
    };
    add("Reset password", () => askPassword(reader));
    if (!isSelf) {
      add(reader.role === "admin" ? "Make reader" : "Make admin", () => changeRole(reader));
      add(reader.disabled ? "Enable" : "Disable", () => toggleDisabled(reader));
      add("Remove", () => removeReader(reader), "text-button danger-text");
    }
    row.append(avatar, who, meta, actions);
    list.append(row);
  });
}

async function patchReader(reader, body, message) {
  try {
    const updated = await request(`/api/v1/admin/users/${reader.id}`, { method: "PATCH", body });
    state.readers = state.readers.map((item) => item.id === updated.id ? updated : item);
    renderReaders();
    showToast(message);
    return true;
  } catch (error) {
    showToast(readableError(error, "The change could not be saved."), true);
    return false;
  }
}

function changeRole(reader) {
  const role = reader.role === "admin" ? "reader" : "admin";
  return confirmAction(`Make ${reader.displayName} ${role === "admin" ? "an administrator" : "a reader"}?`,
    role === "admin" ? "Administrators can manage books, accounts, and settings." : "They will lose access to administration.",
    "Change role", () => patchReader(reader, { role }, `${reader.displayName} is now ${role === "admin" ? "an administrator" : "a reader"}.`));
}

function toggleDisabled(reader) {
  const disabled = !reader.disabled;
  const run = () => patchReader(reader, { disabled }, disabled ? `${reader.displayName} is disabled.` : `${reader.displayName} can sign in again.`);
  if (!disabled) return run();
  return confirmAction(`Disable ${reader.displayName}?`, "They are signed out everywhere and cannot sign in until you enable them again. Reading progress is kept.", "Disable", run);
}

function removeReader(reader) {
  return confirmAction(`Remove ${reader.displayName}?`, "This deletes the account and its reading progress. Book files are not affected. This cannot be undone.", "Remove reader", async () => {
    try {
      await request(`/api/v1/admin/users/${reader.id}`, { method: "DELETE" });
      state.readers = state.readers.filter((item) => item.id !== reader.id);
      renderReaders();
      showToast(`${reader.displayName} was removed.`);
    } catch (error) {
      showToast(readableError(error, "The reader could not be removed."), true);
    }
  });
}

function askPassword(reader) {
  const dialog = $("#password-dialog");
  const form = $("#password-form");
  form.reset();
  setError("password", "");
  $("#password-help").textContent = `Set a new password for ${reader.displayName}. They will be signed out on every device.`;
  form.onsubmit = async (event) => {
    event.preventDefault();
    setError("password", "");
    await withButton(event.submitter, "Saving...", async () => {
      const ok = await patchReader(reader, { password: form.password.value }, `Password reset for ${reader.displayName}.`);
      if (ok) dialog.close();
      else setError("password", "The password could not be reset. Use 12 or more characters.");
    });
  };
  dialog.showModal();
}

function confirmAction(title, body, acceptLabel, action) {
  const dialog = $("#confirm-dialog");
  $("#confirm-title").textContent = title;
  $("#confirm-body").textContent = body;
  $("#confirm-accept").textContent = acceptLabel;
  dialog.onclose = () => { if (dialog.returnValue === "accept") action(); };
  dialog.returnValue = "";
  dialog.showModal();
}

async function loadActivity() {
  $("#activity-status").textContent = "Loading activity...";
  try {
    const { items } = await request("/api/v1/admin/audit?limit=100");
    const list = $("#activity-list");
    list.replaceChildren();
    $("#activity-status").textContent = items.length ? `${items.length} most recent` : "";
    if (!items.length) { list.append(emptyState("Nothing yet.", "Account and book changes will appear here.")); return; }
    const format = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
    items.forEach((entry) => {
      const item = document.createElement("li");
      const time = document.createElement("time");
      time.dateTime = entry.createdAt;
      time.textContent = format.format(new Date(entry.createdAt));
      item.append(textNode("strong", ACTIONS[entry.action] || entry.action), textNode("span", entry.summary), textNode("span", `by ${entry.actorEmail}`, "by"), time);
      list.append(item);
    });
  } catch (error) {
    $("#activity-status").textContent = readableError(error, "Activity could not be loaded.");
  }
}

const ACTIONS = {
  "user.create": "Reader added", "user.update": "Account changed", "user.delete": "Account removed",
  "book.import": "Book imported", "book.update": "Book edited", "book.delete": "Book deleted",
  "export.create": "Export downloaded",
};

async function createReader(event) {
  event.preventDefault();
  setError("reader", "");
  await withButton(event.submitter, "Creating...", async () => {
    try {
      const reader = await request("/api/v1/admin/users", { method: "POST", body: formObject(event.currentTarget) });
      event.currentTarget.reset();
      state.readers.push(reader);
      renderReaders();
      showToast(`${reader.displayName} can now sign in.`);
    } catch (error) {
      setError("reader", readableError(error, "The reader could not be created."));
    }
  });
}

async function request(path, options = {}) {
  const method = options.method || "GET";
  const headers = new Headers(options.headers || {});
  const useAuth = options.auth !== false;
  let body = options.body;
  if (body && !(body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }
  if (useAuth && state.session?.accessToken) headers.set("Authorization", `Bearer ${state.session.accessToken}`);
  let response = await fetch(path, { method, headers, body });
  if (response.status === 401 && useAuth && state.session?.refreshToken && path !== "/api/v1/sessions/refresh") {
    if (await refreshSession()) {
      headers.set("Authorization", `Bearer ${state.session.accessToken}`);
      response = await fetch(path, { method, headers, body });
    }
  }
  if (response.status === 204) return null;
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new APIError(response.status, payload.code, payload.message || `Request failed with status ${response.status}.`);
  return payload;
}

async function refreshSession() {
  try {
    const response = await fetch("/api/v1/sessions/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: state.session.refreshToken }),
    });
    if (!response.ok) throw new Error("refresh failed");
    state.session = await response.json();
    saveSession();
    return true;
  } catch (_) {
    clearSession();
    showLogin();
    setError("login", "Your session expired. Sign in again.");
    return false;
  }
}

class APIError extends Error {
  constructor(status, code, message) { super(message); this.status = status; this.code = code; }
}

function readableError(error, fallback) { return error instanceof APIError ? error.message : fallback; }
function formObject(form) { return Object.fromEntries(new FormData(form).entries()); }
function setError(name, message) { $(`[data-error="${name}"]`).textContent = message; }
function readSession() { try { return JSON.parse(localStorage.getItem("bookharbor.admin.session")); } catch (_) { return null; } }
function saveSession() { localStorage.setItem("bookharbor.admin.session", JSON.stringify(state.session)); }
function clearSession() { state.session = null; localStorage.removeItem("bookharbor.admin.session"); }

async function withButton(button, busyLabel, work) {
  if (!button) return work();
  const label = button.textContent;
  button.disabled = true;
  button.textContent = busyLabel;
  try { return await work(); } finally { button.disabled = false; button.textContent = label; }
}

function showToast(message, isError = false) {
  const toast = $("#toast");
  toast.textContent = message;
  toast.classList.toggle("error", isError);
  toast.hidden = false;
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => toast.hidden = true, 4200);
}

function node(tag, className) { const element = document.createElement(tag); if (className) element.className = className; return element; }
function textNode(tag, text, className) { const element = node(tag, className); element.textContent = text; return element; }
function emptyState(title, message) { const element = node("div", "empty-state"); element.append(textNode("strong", title), document.createTextNode(message)); return element; }
function initials(value) { return String(value || "BH").split(/\s+/).slice(0, 2).map((word) => word[0]).join("").toUpperCase(); }
