"use strict";

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => [...document.querySelectorAll(selector)];
const icon = (name) => `<svg class="icon" aria-hidden="true"><use href="#i-${name}"/></svg>`;
let state = { settings: { nickname: "", voice: false, volume: 70, speechLanguage: "zh" }, sessions: [], memories: [], companions: [], activeCompanionId: "" };
let config = null;
let currentSessionID = "";
let currentView = "home";
let pending = null;
let toastTimer;
let recording = null;
let transcribing = null;
let recordingTimer = null;
let mediaGeneration = 0;
let speechRequest = null;
let speaking = false;
let currentSong = null;
let confirmResolver = null;
let touchedUntil = 0;
let touchIndex = 0;
let captureRequest = 0;
let companionImageURL = "";
let companionImageRequest = null;
let companionImageGeneration = 0;
let savingCompanionVoice = false;
const expressions = {
  idle: [1, 0], listening: [1, 0], thinking: [3, 2],
  speaking: [0, 0], singing: [2, 2], love: [3, 1],
  shy: [0, 2], cheer: [1, 3], thanks: [3, 3],
};

function setExpression(name) {
  const [column, row] = expressions[name] || expressions.idle;
  const top = [0, 199, 389, 578][row];
  const img = $("#character-sheet");
  img.style.left = `${column * -100}%`;
  img.style.top = `${-top / 175 * 100}%`;
  $("#whale-character").dataset.expression = name;
}
const songAudio = $("#song-audio");
const replyAudio = $("#reply-audio");
const speechURLs = new Map();
let transientSpeechURL = null;

function toast(message) {
  const el = $("#toast");
  el.textContent = typeof message === "string" ? message : message?.message || "刚才没有完成，请再试一次。";
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 4700);
}

async function api(path, options = {}) {
  const init = { ...options, headers: { ...options.headers } };
  if (init.body && !(init.body instanceof FormData)) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(init.body);
  }
  let response;
  try { response = await fetch(path, init); }
  catch (error) {
    if (error.name === "AbortError") throw error;
    throw new Error("本地服务没有回应，请确认鲸伴已经启动。");
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    const error = new Error(body.error || "刚才没有完成，请稍后重试。");
    error.status = response.status;
    throw error;
  }
  return response.json();
}

function node(tag, className, text) {
  const el = document.createElement(tag);
  if (className) el.className = className;
  if (text !== undefined) el.textContent = text;
  return el;
}

function iconButton(name, label, action, className = "icon-button") {
  const el = node("button", className);
  el.type = "button";
  el.innerHTML = icon(name);
  el.title = label;
  el.setAttribute("aria-label", label);
  el.addEventListener("click", () => Promise.resolve(action()).catch(toast));
  return el;
}

function session() { return state.sessions.find((item) => item.id === currentSessionID); }
function currentCompanion() { return state.companions.find((item) => item.id === (session()?.companionId || state.activeCompanionId)) || state.companions[0] || config?.character || { name: "大肥鱼", greeting: "嗨，我是大肥鱼。今天想聊些什么？" }; }
function portraitSource(companion = currentCompanion()) { return companion.imageUrl && companion.imageUrl !== "/assets/whale-girl-sheet.png" ? companion.imageUrl : ""; }
const localVoiceIDs = ["anon", "soyo", "sakiko_white"];
function localVoiceOptions() {
  const names = { anon: "千早爱音", soyo: "长崎素世", sakiko_white: "客服小祥" };
  const catalog = Array.isArray(config?.voices) ? config.voices : [];
  return localVoiceIDs.map((id) => {
    const item = catalog.find((voice) => voice.id === id);
    return { id, name: item?.name || names[id], available: item ? item.available !== false : id === "anon" && Boolean(config?.anonVoiceAvailable) };
  });
}
function voiceOptions() { return [{ id: "default", name: "跟随默认音色", available: true }, ...localVoiceOptions(), { id: "browser", name: "设备朗读", available: true }]; }
function savedVoice() { return currentCompanion().voice || "default"; }
function previewVoice() { return $("#settings-companion-voice")?.value || savedVoice(); }
function resolvedVoice(choice = savedVoice()) {
  return localVoiceIDs.includes(choice) ? "gpt-sovits" : choice === "browser" ? "browser" : config?.serverTTS ? config.ttsProvider : "browser";
}
function voiceName(choice = savedVoice()) {
  if (choice === "browser") return "设备朗读";
  if (choice === "default") {
    if (localVoiceIDs.includes(config?.defaultVoice)) return localVoiceOptions().find((voice) => voice.id === config.defaultVoice).name;
    return resolvedVoice(choice) === "gpt-sovits" ? "默认本地音色" : resolvedVoice(choice) === "browser" ? "设备朗读" : "默认语音服务";
  }
  return localVoiceOptions().find((voice) => voice.id === choice)?.name || "所选音色";
}
function voiceAvailable(choice) { return !localVoiceIDs.includes(choice) || Boolean(localVoiceOptions().find((voice) => voice.id === choice)?.available); }
function fillVoiceSelect(select, selected) {
  select.replaceChildren();
  for (const voice of voiceOptions()) {
    const option = node("option", "", voice.name + (voice.available ? "" : "（未就绪）"));
    option.value = voice.id; option.disabled = !voice.available; select.append(option);
  }
  select.value = voiceOptions().some((voice) => voice.id === selected) ? selected : "default";
}
function renderVoiceSettings() {
  const companion = currentCompanion();
  const select = $("#settings-companion-voice");
  const changedCompanion = select.dataset.companionId !== companion.id;
  const choice = changedCompanion ? savedVoice() : select.value || savedVoice();
  fillVoiceSelect(select, choice);
  select.dataset.companionId = companion.id || "";
  select.disabled = savingCompanionVoice;
  $("#save-companion-voice").disabled = savingCompanionVoice || select.value === savedVoice() || !voiceAvailable(select.value);
  if (changedCompanion) $("#companion-voice-status").textContent = `已保存：${voiceName()}。更改后可先试听，保存音色后用于聊天回复。`;
}
function speechLanguage() { return state.settings.speechLanguage || "zh"; }
function languageLabel(language) { return language === "yue" ? "粤语" : "普通话"; }
function updateVoiceNotes() {
  const preview = previewVoice();
  const provider = resolvedVoice(preview);
  const selectedLanguage = $("#speech-language")?.value || speechLanguage();
  const draft = preview !== savedVoice();
  $("#voice-service-note").textContent = `聊天回复：${voiceName()} · ${languageLabel(speechLanguage())}。`;
  $("#voice-preview-note").textContent = `试听${draft ? "未保存的选择" : "当前音色"}：${voiceName(preview)} · ${languageLabel(selectedLanguage)}。` + (provider === "gpt-sovits" ? "首次切换音色可能需要稍等片刻。" : "无需开启自动朗读。");
  $("#connection-copy").textContent = `${config?.chatMode === "live" ? "AI 对话" : "本地演示回复"} · ${config?.serverASR ? "录音识别" : "浏览器识别"} · ${voiceName()} · ${languageLabel(speechLanguage())}`;
}
async function saveCompanionVoice() {
  if (savingCompanionVoice) return;
  const companion = currentCompanion();
  if (!companion.id) { toast("伴侣还未加载，请稍后重试。"); return; }
  const choice = previewVoice();
  if (!voiceAvailable(choice)) { toast("这个音色的资源还未就绪，请选择其他音色。"); return; }
  if (choice === savedVoice()) return;
  stopReplyAudio(); cleanSpeechCache();
  savingCompanionVoice = true; renderVoiceSettings();
  $("#companion-voice-status").textContent = "正在保存音色…";
  try {
    const body = { name: companion.name, identity: companion.identity, personality: companion.personality, speakingStyle: companion.speakingStyle, greeting: companion.greeting, theme: companion.theme, imageUrl: companion.imageUrl, voice: choice };
    const updated = await api(`/api/companions/${companion.id}`, { method: "PUT", body });
    state.companions = state.companions.map((item) => item.id === updated.id ? updated : item);
    if (currentCompanion().id === updated.id) {
      $("#settings-companion-voice").value = updated.voice || "default";
      $("#companion-voice-status").textContent = `已保存：${voiceName(updated.voice)}。接下来的聊天回复会使用这个声音。`;
    }
    toast(`已为${updated.name}保存${voiceName(updated.voice)}音色。`);
  } catch (error) {
    if (currentCompanion().id === companion.id) $("#companion-voice-status").textContent = "音色没有保存成功，可以重新尝试。";
    toast(error);
  } finally {
    savingCompanionVoice = false; renderVoiceSettings(); updateVoiceNotes();
  }
}
function formatTime(seconds) { seconds = Math.max(0, Math.floor(seconds || 0)); return `${String(Math.floor(seconds / 60)).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`; }
function shortTime(date) { return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(date)); }
function persistSessionID() { try { localStorage.setItem("whale.currentSession", currentSessionID); } catch { /* Storage can be disabled. */ } }

function switchView(view) {
  currentView = view;
  const titles = { home: "陪伴空间", music: "音乐小屋", memories: "记忆瓶", settings: "偏好设置" };
  $$(".view").forEach((el) => { el.hidden = el.id !== `view-${view}`; });
  $$(".nav-item").forEach((el) => { el.classList.toggle("active", el.dataset.view === view); if (el.dataset.view === view) el.setAttribute("aria-current", "page"); else el.removeAttribute("aria-current"); });
  $("#page-title").textContent = titles[view];
  if (view === "settings") renderSettings();
  if (view === "memories") renderMemories();
  if (window.innerWidth < 901) window.scrollTo({ top: 0, behavior: "smooth" });
}

function renderHistory() {
  const target = $("#history-list");
  target.replaceChildren();
  const sorted = [...state.sessions].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
  if (!sorted.length) target.append(node("p", "empty-history", "第一段故事，等你开口。"));
  for (const item of sorted) {
    const row = node("div", `history-item${item.id === currentSessionID ? " current" : ""}`);
    const open = node("button", "history-open");
    open.innerHTML = icon("chat");
    const companion = state.companions.find((c) => c.id === item.companionId);
    open.append(node("span", "", `${companion?.name || "大肥鱼"} · ${item.title}`));
    open.title = `${companion?.name || "大肥鱼"} · ${item.title}`;
    open.addEventListener("click", () => {
      cancelPending(); cancelRecording(); stopPlayback(); currentSessionID = item.id; persistSessionID();
      renderHistory(); renderMessages(); switchView("home");
    });
    row.append(open, iconButton("trash", `删除对话：${item.title}`, async () => {
      if (!await confirmAction("删除这段小日常？", "这段聊天会被删除，记忆瓶里的内容会保留。")) return;
      if (item.id === currentSessionID) { cancelPending(); cancelRecording(); stopPlayback(); }
      await api(`/api/sessions/${item.id}`, { method: "DELETE" });
      state.sessions = state.sessions.filter((s) => s.id !== item.id);
      if (currentSessionID === item.id) { currentSessionID = state.sessions[0]?.id || ""; if (!currentSessionID) await createSession(); }
      persistSessionID(); renderHistory(); renderMessages(); toast("这段聊天已删除，记忆瓶没有变化。");
    }, "history-delete"));
    target.append(row);
  }
}

function scrollChat() { const el = $("#chat-scroll"); requestAnimationFrame(() => { el.scrollTop = el.scrollHeight; }); }

function renderMessages() {
  renderCompanion();
  const target = $("#messages");
  target.replaceChildren();
  const messages = session()?.messages || [];
  const name = state.settings.nickname;
  const greeting = `${name ? `${name}，` : ""}${currentCompanion().greeting}`;
  appendMessage(target, { id: "greeting", role: "assistant", content: greeting, status: "complete", createdAt: session()?.createdAt || new Date().toISOString() });
  for (const message of messages) appendMessage(target, message);
  $("#suggestions").hidden = messages.length > 1;
  updatePendingUI(); scrollChat();
}

function appendMessage(target, message) {
  const isUser = message.role === "user";
  const row = node("article", `message ${isUser ? "user" : "assistant"}`);
  row.dataset.messageId = message.id;
  const avatar = node("span", "message-avatar");
  if (isUser) avatar.textContent = (state.settings.nickname || "你").slice(0, 1);
  else { const img = node("img"); const portrait = portraitSource(); img.src = portrait || "/assets/whale-girl-sheet.png"; img.alt = ""; avatar.classList.toggle("portrait-avatar", Boolean(portrait)); avatar.append(img); }
  const body = node("div", "message-body");
  const label = node("div", "message-label");
  label.append(node("span", "", isUser ? (state.settings.nickname || "你") : currentCompanion().name), node("time", "", shortTime(message.createdAt)));
  body.append(label, node("div", "message-bubble", message.content));
  if (message.songIds?.length) {
    for (const id of message.songIds) {
      const song = config.songs.find((s) => s.id === id);
      if (!song) continue;
      const play = node("button", "song-chip");
      play.innerHTML = icon("play"); play.append(node("span", "", `${song.title} · 合成哼唱`));
      play.addEventListener("click", () => playSong(song).catch(toast));
      body.append(play);
    }
  }
  const actions = node("div", "message-actions");
  if (!isUser) {
    const speak = iconButton("volume", "朗读这条回复", () => speakText(message.content));
    speak.append(node("span", "", "听一听")); actions.append(speak);
  } else if (message.status !== "pending") {
    const save = iconButton("bottle", "保存为记忆", () => openMemory(message.content));
    save.append(node("span", "", "放进记忆瓶")); actions.append(save);
  }
  if (["failed", "cancelled"].includes(message.status)) {
    actions.append(node("span", "message-status", message.status === "cancelled" ? "已取消" : "回复未完成"));
    const retry = node("button", "", "重试");
    retry.addEventListener("click", () => sendMessage(message.content, message.requestId)); actions.append(retry);
  }
  if (actions.childNodes.length) body.append(actions);
  row.append(avatar, body); target.append(row);
}

function updateCharacter() {
  let mode = "idle";
  let caption = "在这里陪你";
  let bubble = "你来啦，给你留了个位置。";
  if (recording) { mode = "listening"; caption = "认真听你说"; bubble = "嗯嗯，我在听呢…"; }
  else if (transcribing) { mode = "thinking"; caption = "正在整理你的话"; bubble = "让我听清楚一点…"; }
  else if (speechRequest) { mode = "thinking"; caption = "正在合成声音"; bubble = "正在准备声音，稍等一下。"; }
  else if (pending) { mode = "thinking"; caption = "正在想怎么回应"; bubble = "稍等，让我想一小下。"; }
  else if (currentSong && !songAudio.paused) { mode = "singing"; caption = "陪你听一首小曲"; bubble = "这三十六秒，留给音乐 ♪"; }
  else if (speaking) { mode = "speaking"; caption = "正在和你说话"; bubble = "把小小的心情，慢慢说给你听。"; }
  $("#companion-card").dataset.state = mode;
  $("#character-status").textContent = caption;
  $("#character-bubble").textContent = bubble;
  if (mode === "idle" && Date.now() < touchedUntil) {
    const moods = ["love", "shy", "cheer", "thanks"];
    setExpression(moods[(touchIndex - 1) % moods.length]);
    $("#character-bubble").textContent = ["嘿嘿，收到你的摸摸啦。", "诶？我有一点点害羞…", "今天也要和你一起元气满满！", "谢谢你来陪我呀。"][(touchIndex - 1) % moods.length];
  } else setExpression(mode);
  $("#stop-voice").hidden = !speaking && !speechRequest;
  $("#voice-preview-stop").hidden = !speaking && !speechRequest;
  $("#voice-preview").disabled = Boolean(speechRequest || pending || recording || transcribing);
  $("#voice-preview-status").textContent = speechRequest ? "正在合成声音…" : speaking ? "正在播放" : "";
}

function updatePendingUI() {
  $("#typing").hidden = !pending || pending.sessionID !== currentSessionID;
  $("#send-button").hidden = Boolean(pending);
  $("#cancel-chat").hidden = !pending;
  $("#send-button").disabled = Boolean(recording || transcribing || pending);
  $("#mic-button").disabled = Boolean(pending || transcribing);
  $$("[data-prompt]").forEach((el) => { el.disabled = Boolean(pending || recording || transcribing); });
  updateCharacter();
}

async function createSession(companionId = currentCompanion().id || state.activeCompanionId) {
  const item = await api("/api/sessions", { method: "POST", body: { companionId } });
  state.sessions.unshift(item); currentSessionID = item.id; persistSessionID(); return item;
}

async function newSession() {
  cancelPending(); cancelRecording(); stopPlayback();
  try { await createSession(); renderHistory(); renderMessages(); switchView("home"); $("#message-input").value = ""; $("#message-input").focus(); }
  catch (error) { toast(error); }
}

async function sendMessage(content, existingRequestID = null) {
  content = content.trim();
  if (!content || pending || recording || transcribing) return;
  if ([...content].length > 1000) { toast("这句话有点长，可以分成两段说。每条最多 1000 个字。"); return; }
  stopPlayback();
  if (!currentSessionID) { try { await createSession(); } catch (error) { toast(error); return; } }
  const requestID = existingRequestID || crypto.randomUUID();
  const sessionID = currentSessionID;
  const controller = new AbortController();
  const current = session();
  let userMessage = current.messages.find((msg) => msg.role === "user" && msg.requestId === requestID);
  if (userMessage) userMessage.status = "pending";
  else {
    userMessage = { id: requestID, requestId: requestID, role: "user", content, status: "pending", createdAt: new Date().toISOString() };
    current.messages.push(userMessage);
  }
  const task = { requestID, sessionID, controller };
  pending = task; current.updatedAt = new Date().toISOString();
  if (current.title === "新的小日常") current.title = [...content].slice(0, 18).join("") + ([...content].length > 18 ? "…" : "");
  $("#message-input").value = ""; $("#input-hint").textContent = "";
  renderHistory(); renderMessages();
  try {
    const reply = await api("/api/chat", { method: "POST", body: { sessionId: sessionID, content, requestId: requestID }, signal: controller.signal });
    if (pending !== task || controller.signal.aborted) return;
    userMessage.status = "complete";
    current.messages.splice(current.messages.indexOf(userMessage) + 1, 0, reply);
    pending = null;
    renderMessages();
    if (state.settings.voice) speakText(reply.content).catch(toast);
  } catch (error) {
    if (pending !== task) return;
    userMessage.status = error.name === "AbortError" ? "cancelled" : "failed";
    if (error.name !== "AbortError") toast(error);
  } finally {
    if (pending === task) pending = null;
    if (currentSessionID === sessionID) renderMessages();
    updatePendingUI();
  }
}

function cancelPending() {
  if (!pending) return;
  const task = pending;
  pending = null; task.controller.abort();
  const old = state.sessions.find((s) => s.id === task.sessionID);
  const message = old?.messages.find((m) => m.requestId === task.requestID && m.role === "user");
  if (message) message.status = "cancelled";
  renderMessages(); updatePendingUI();
}

function renderMusic() {
  const target = $("#music-grid"); target.replaceChildren();
  config.songs.forEach((song, index) => {
    const card = node("article", "music-card");
    const art = node("div", `music-art ${song.theme}`);
    art.append(node("small", "", "A LITTLE WHALE MELODY"));
    const disc = node("div", "disc"); disc.innerHTML = icon(song.theme === "moon" ? "spark" : "music");
    art.append(disc, node("span", "song-no", `0${index + 1}`));
    const body = node("div", "music-card-body"); body.append(node("h2", "", song.title), node("p", "", song.subtitle));
    const meta = node("div", "song-meta"); meta.append(node("small", "", `合成哼唱示例 · ${formatTime(song.duration)}`), iconButton("play", `播放${song.title}`, () => playSong(song), "song-play"));
    body.append(meta); card.append(art, body); target.append(card);
  });
}

function stopReplyAudio() {
  mediaGeneration++;
  if (speechRequest) { speechRequest.abort(); speechRequest = null; }
  if ("speechSynthesis" in window) speechSynthesis.cancel();
  replyAudio.pause(); replyAudio.removeAttribute("src"); replyAudio.load();
  if (transientSpeechURL) { URL.revokeObjectURL(transientSpeechURL); transientSpeechURL = null; }
  speaking = false;
  updateCharacter();
}

function stopPlayback() {
  stopReplyAudio();
  songAudio.pause(); songAudio.currentTime = 0;
  currentSong = null;
  $("#mini-player").hidden = true;
  updateCharacter();
}

async function playSong(song) {
  if (recording || transcribing || pending) { toast("先结束当前的录音或对话，再听一首小曲吧。"); return; }
  stopPlayback();
  currentSong = song;
  const generation = mediaGeneration;
  songAudio.src = song.url;
  songAudio.volume = state.settings.volume / 100;
  $("#player-title").textContent = song.title;
  $("#player-art").className = `player-art ${song.theme}`;
  $("#player-progress").max = String(song.duration);
  $("#player-progress").value = "0";
  $("#mini-player").hidden = false;
  updatePlayer();
  try {
    await songAudio.play();
    if (generation !== mediaGeneration) return;
    updatePlayer(); updateCharacter();
  } catch (error) {
    if (generation !== mediaGeneration) return;
    updatePlayer(); updateCharacter();
    if (error.name === "NotAllowedError") toast("浏览器暂停了自动播放，点播放器上的播放按钮就好。");
    else { stopPlayback(); toast("这首小曲暂时没有加载成功，请再试一次。"); }
  }
}

function updatePlayer() {
  const paused = songAudio.paused;
  $("#player-toggle").innerHTML = icon(paused ? "play" : "pause");
  $("#player-toggle").setAttribute("aria-label", paused ? "继续播放歌曲" : "暂停歌曲");
  $("#player-time").textContent = `${formatTime(songAudio.currentTime)} / ${formatTime(songAudio.duration || currentSong?.duration || 36)}`;
  $("#player-progress").value = String(songAudio.currentTime || 0);
  if (Number.isFinite(songAudio.duration)) $("#player-progress").max = String(songAudio.duration);
}

async function speakText(text, volume = state.settings.volume, language = speechLanguage(), voiceOverride) {
  text = text.trim();
  if (!text) { toast("先输入一句想试听的话。"); return; }
  if (recording || transcribing || pending) { toast("先结束当前操作，再听这一条吧。"); return; }
  stopPlayback();
  const generation = mediaGeneration;
  const choice = voiceOverride === undefined ? savedVoice() : voiceOverride;
  if (!voiceAvailable(choice)) { toast(`${voiceName(choice)}的资源还未就绪，请在偏好中选择其他音色。`); return; }
  const provider = resolvedVoice(choice);
  if (provider !== "browser") {
    const realtime = provider === "gpt-sovits";
    const cacheKey = `${currentCompanion().id}:${choice}:${language}:${text}`;
    let url = realtime ? null : speechURLs.get(cacheKey);
    if (!url) {
      const controller = new AbortController(); speechRequest = controller; updateCharacter();
      try {
        const res = await fetch("/api/speech", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text, companionId: currentCompanion().id, language, voice: voiceOverride }), signal: controller.signal });
        if (!res.ok) { const body = await res.json().catch(() => ({})); throw new Error(body.error || "声音暂时没有生成成功，文字仍然保留。"); }
        const blob = await res.blob();
        if (generation !== mediaGeneration) return;
        url = URL.createObjectURL(blob);
        if (realtime) transientSpeechURL = url; else speechURLs.set(cacheKey, url);
        if (speechURLs.size > 20) { const oldest = speechURLs.keys().next().value; URL.revokeObjectURL(speechURLs.get(oldest)); speechURLs.delete(oldest); }
      } catch (error) { if (error.name !== "AbortError" && generation === mediaGeneration) toast(error); return; }
      finally { if (speechRequest === controller) speechRequest = null; updateCharacter(); }
    }
    if (generation !== mediaGeneration) return;
    replyAudio.src = url; replyAudio.volume = volume / 100;
    try { await replyAudio.play(); }
    catch (error) { if (generation === mediaGeneration) toast(error.name === "NotAllowedError" ? "点一下这条消息的“听一听”，就能播放声音。" : "声音播放失败，文字回复仍然保留。"); }
    return;
  }
  if (!("speechSynthesis" in window)) { toast("这个浏览器还不能朗读，可以先看文字回复。"); return; }
  let voices = speechSynthesis.getVoices();
  if (!voices.length) {
    await new Promise((resolve) => { const timeout = setTimeout(resolve, 700); speechSynthesis.addEventListener("voiceschanged", () => { clearTimeout(timeout); resolve(); }, { once: true }); });
    if (generation !== mediaGeneration) return;
    voices = speechSynthesis.getVoices();
  }
  const voice = language === "yue" ? voices.find((v) => /^(yue|zh[-_](HK|MO))/i.test(v.lang)) : voices.find((v) => /^(cmn|zh[-_](CN|TW)|zh$)/i.test(v.lang));
  if (!voice) { toast(`这台设备没有可用的${languageLabel(language)}声音。可以在偏好中选择一个本地角色音色后重试。`); return; }
  const utterance = new SpeechSynthesisUtterance(text);
  utterance.voice = voice; utterance.lang = language === "yue" ? "zh-HK" : "zh-CN"; utterance.rate = 0.94; utterance.pitch = 1.12; utterance.volume = volume / 100;
  utterance.onstart = () => { if (generation === mediaGeneration) { speaking = true; updateCharacter(); } };
  utterance.onend = () => { if (generation === mediaGeneration) { speaking = false; updateCharacter(); } };
  utterance.onerror = (event) => { if (generation === mediaGeneration) { speaking = false; updateCharacter(); if (!["canceled", "interrupted"].includes(event.error)) toast("这次没有念出声音，你可以再点一次“听一听”。"); } };
  speechSynthesis.speak(utterance);
}

function renderMemories() {
  $("#memory-count").textContent = String(state.memories.length);
  const target = $("#memory-grid"); target.replaceChildren();
  if (!state.memories.length) {
    const empty = node("div", "empty-memory"); empty.innerHTML = icon("bottle");
    empty.append(node("h2", "", "记忆瓶还是空的"), node("p", "", "留下一件关于你的小事，让下一次聊天更熟悉。")); target.append(empty);
  }
  for (const memory of state.memories) {
    const card = node("article", "memory-card");
    const top = node("div", "memory-card-top"); top.append(node("span", "memory-category", memory.category));
    const sparkle = node("span"); sparkle.innerHTML = icon("spark"); top.append(sparkle);
    const footer = node("div", "memory-card-footer"); footer.append(node("span", "", new Date(memory.updatedAt).toLocaleDateString("zh-CN")));
    const actions = node("div");
    actions.append(iconButton("edit", "编辑这条记忆", () => openMemory(memory.content, memory)), iconButton("trash", "删除这条记忆", async () => {
      if (!await confirmAction("让这件小事离开记忆瓶？", "删除后，伴侣不会再从记忆瓶带上这条内容。已有聊天里的文字会保留，可以新建对话。")) return;
      await api(`/api/memories/${memory.id}`, { method: "DELETE" }); state.memories = state.memories.filter((m) => m.id !== memory.id); renderMemories(); toast("这条记忆已经删除。");
    }));
    footer.append(actions); card.append(top, node("p", "", memory.content), footer); target.append(card);
  }
}

function openMemory(content = "", memory = null) {
  $("#memory-id").value = memory?.id || "";
  $("#memory-content").value = content.slice(0, 300);
  $("#memory-category").value = memory?.category || "小偏好";
  $("#memory-dialog-title").textContent = memory ? "整理这件小事" : "留下一件小事";
  $("#memory-dialog").showModal();
  $("#memory-content").focus();
}

function confirmAction(title, copy) {
  $("#confirm-title").textContent = title; $("#confirm-copy").textContent = copy;
  $("#confirm-dialog").showModal();
  return new Promise((resolve) => { confirmResolver = resolve; });
}
function finishConfirm(accepted) { $("#confirm-dialog").close(); if (confirmResolver) { const resolve = confirmResolver; confirmResolver = null; resolve(accepted); } }

function renderSettings() {
  renderCompanion();
  $("#nickname").value = state.settings.nickname;
  $("#setting-voice").checked = state.settings.voice;
  $("#volume").value = String(state.settings.volume);
  $("#speech-language").value = speechLanguage();
  const previewSamples = ["你好呀，今天也想陪你聊聊天。", "你好呀，今日都想陪你倾下偈。"];
  if (previewSamples.includes($("#voice-preview-text").value)) $("#voice-preview-text").value = previewSamples[speechLanguage() === "yue" ? 1 : 0];
  updateVoiceNotes();
  $("#volume-value").textContent = `${state.settings.volume}%`;
  $("#settings-status").textContent = "";
  $("#sidebar-name").textContent = state.settings.nickname || "海边的朋友";
  $("#user-avatar").textContent = (state.settings.nickname || "你").slice(0, 1);
  $("#voice-toggle").setAttribute("aria-pressed", String(state.settings.voice));
  $("#voice-toggle").innerHTML = icon(state.settings.voice ? "volume" : "mute") + "<span></span>";
  $("#voice-toggle span").textContent = `听${currentCompanion().name}说话`;
}

function renderCompanion() {
  const companion = currentCompanion();
  $("#character-name").textContent = companion.name;
  $("#character-name").title = companion.name;
  const portrait = portraitSource(companion);
  const theme = companion.theme || "ocean";
  document.body.dataset.theme = theme;
  document.body.classList.toggle("has-portrait", Boolean(portrait));
  $("#companion-card").classList.toggle("has-portrait", Boolean(portrait));
  $("#character-portrait").hidden = !portrait;
  $("#whale-character").hidden = Boolean(portrait);
  const portraitTouch = $("#portrait-touch");
  if (portraitTouch) { portraitTouch.hidden = !portrait; portraitTouch.setAttribute("aria-label", `和${companion.name}打个招呼`); }
  if (portrait) { $("#character-portrait").src = portrait === "/assets/anon.png" ? "/assets/anon-poster.png" : portrait; $("#character-portrait").alt = companion.name + "的参考形象"; }
  else $("#character-portrait").removeAttribute("src");
  for (const id of ["brand-portrait", "chat-portrait"]) {
    const img = $("#" + id);
    if (img) { img.src = portrait || "/assets/whale-girl-sheet.png"; img.parentElement.classList.toggle("portrait-avatar", Boolean(portrait)); }
  }
  const anon = theme === "anon";
  if ($("#brand-name")) $("#brand-name").textContent = anon ? "ANON ROOM" : "鲸伴";
  if ($("#brand-kicker")) $("#brand-kicker").textContent = anon ? "A LITTLE MOMENT WITH YOU" : "A LITTLE ROOM FOR YOU";
  if ($("#room-name")) $("#room-name").textContent = `${companion.name}的陪伴空间`;
  document.title = `${companion.name} · 鲸伴`;
  $("meta[name=theme-color]").content = anon ? "#fff2f7" : "#f5f8fb";
  const copy = {
    "hero-eyebrow": anon ? "ANON'S LITTLE ROOM" : "YOUR LITTLE COMPANION",
    "hero-title": anon ? "今天的故事，\n说给我听吧。" : "今天，也想陪着你。",
    "hero-copy": anon ? "练习结束后的空隙，刚好留给你。" : "开心的小事，或是没来由的心情，都可以。",
    "hero-footnote": anon ? "再小的日常，也值得一起期待。" : "不必急着找到答案，先让心情靠靠岸。",
  };
  for (const [id, value] of Object.entries(copy)) if ($("#" + id)) $("#" + id).textContent = value;
  $("#character-identity").textContent = companion.identity || "你的专属伙伴";
  $("#chat-title").textContent = `和${companion.name}聊聊`;
  $("#typing-label").textContent = `${companion.name}正在想怎么回答…`;
  $("#message-input").placeholder = `和${companion.name}说说话吧…`;
  $("#message-input").setAttribute("aria-label", `给${companion.name}发消息`);
  $("#whale-character").setAttribute("aria-label", `摸摸${companion.name}，切换一个表情`);
  $("#voice-toggle span").textContent = `听${companion.name}说话`;
  const selector = $("#companion-select");
  selector.replaceChildren();
  for (const item of state.companions) { const option = node("option", "", item.name); option.value = item.id; selector.append(option); }
  selector.value = companion.id || "";
  $("#companion-summary").textContent = `${companion.identity || ""}；${companion.personality || ""}；${companion.speakingStyle || ""}`;
  renderVoiceSettings();
  updateVoiceNotes();
}

function openCompanion(companion = null) {
  $("#companion-id").value = companion?.id || "";
  $("#companion-name").value = companion?.name || "";
  $("#companion-identity").value = companion?.identity || "";
  $("#companion-personality").value = companion?.personality || "";
  $("#companion-speaking-style").value = companion?.speakingStyle || "";
  $("#companion-greeting").value = companion?.greeting || "";
  $("#companion-theme").value = companion?.theme || "ocean";
  fillVoiceSelect($("#companion-voice"), companion?.voice || "default");
  cancelCompanionImageUpload();
  companionImageURL = companion?.imageUrl || "";
  $("#companion-image").value = "";
  $("#companion-image-status").textContent = "";
  renderCompanionImage();
  $("#companion-dialog-title").textContent = companion ? "编辑伴侣设定" : "添加一位伴侣";
  $("#companion-dialog").showModal();
  $("#companion-name").focus();
}

function renderCompanionImage() {
  const img = $("#companion-image-preview");
  img.hidden = !companionImageURL;
  if (companionImageURL) img.src = companionImageURL; else img.removeAttribute("src");
  $("#companion-image-remove").hidden = !companionImageURL;
  $("#companion-image-remove").textContent = "恢复默认形象";
  if ($("#companion-image-url")) $("#companion-image-url").value = companionImageURL;
}
function cancelCompanionImageUpload() {
  companionImageGeneration++;
  companionImageRequest?.abort(); companionImageRequest = null;
  $("#companion-form button[type=submit]").disabled = false;
}
async function uploadCompanionImage(file) {
  if (!file) return;
  cancelCompanionImageUpload();
  if (file.size > 5 * 1024 * 1024 || !["image/png", "image/jpeg", "image/webp"].includes(file.type)) { $("#companion-image").value = ""; $("#companion-image-status").textContent = "未替换参考图，请重新选择。"; toast("请选择 5 MB 以内的 PNG、JPG 或 WebP 图片。"); return; }
  const generation = companionImageGeneration;
  const controller = new AbortController(); companionImageRequest = controller;
  $("#companion-form button[type=submit]").disabled = true;
  $("#companion-image-status").textContent = "正在保存参考图…";
  try {
    const form = new FormData(); form.append("image", file);
    const result = await api("/api/companion-images", { method: "POST", body: form, signal: controller.signal });
    if (generation !== companionImageGeneration) return;
    companionImageURL = result.imageUrl;
    renderCompanionImage();
    $("#companion-image-status").textContent = "参考图已就绪，保存伴侣后生效。";
  } catch (error) { if (error.name !== "AbortError" && generation === companionImageGeneration) { $("#companion-image-status").textContent = "参考图未能保存，请重新选择。"; toast(error); } }
  finally { if (generation === companionImageGeneration) { companionImageRequest = null; $("#companion-form button[type=submit]").disabled = false; } }
}

async function chooseCompanion(id) {
  cancelPending(); cancelRecording(); stopPlayback();
  const result = await api(`/api/companions/${id}/select`, { method: "POST" });
  state.activeCompanionId = result.activeCompanionId;
  await createSession(id);
  renderHistory(); renderMessages();
  switchView("home");
  $("#message-input").value = "";
  toast(`已经为你和${currentCompanion().name}开启新对话。`);
}

async function saveSettings(settings, announce = true) {
  state.settings = await api("/api/settings", { method: "PUT", body: settings });
  songAudio.volume = settings.volume / 100; replyAudio.volume = settings.volume / 100;
  if (!settings.voice) stopReplyAudio();
  renderSettings();
  if (announce) { $("#settings-status").textContent = "偏好已保存"; toast("已经按你的偏好整理好啦。"); }
}

function cleanSpeechCache() { for (const url of speechURLs.values()) URL.revokeObjectURL(url); speechURLs.clear(); }

function clearRecordingUI() {
  clearInterval(recordingTimer); recordingTimer = null;
  $("#recording-banner").hidden = true;
  $("#mic-button").classList.remove("recording");
  $("#mic-button").setAttribute("aria-label", "开始语音输入");
  $("#mic-button").title = "开始语音输入";
  $("#mic-button").innerHTML = icon("mic");
  updatePendingUI();
}

function cancelRecording() {
  captureRequest++;
  if (transcribing) { transcribing.abort(); transcribing = null; }
  if (recording) {
    const task = recording; task.cancelled = true; recording = null;
    task.recognition?.abort();
    if (task.recorder?.state === "recording") task.recorder.stop();
    task.stream?.getTracks().forEach((track) => track.stop());
  }
  clearRecordingUI();
}

async function toggleRecording() {
  if (pending || transcribing) return;
  if (recording) { finishRecording(); return; }
  if (!navigator.mediaDevices?.getUserMedia) { toast("当前页面不能使用麦克风，请用本机地址或 HTTPS 打开。也可以直接打字。"); return; }
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!config.serverASR && !SpeechRecognition) { toast("这个浏览器暂不支持语音识别。可换 Chrome 或 Edge，或连接识别服务，文字输入仍可使用。"); return; }
  if (config.serverASR && !("MediaRecorder" in window)) { toast("当前浏览器不能录音，可以先用文字聊天。"); return; }
  stopPlayback();
  const captureID = ++captureRequest;
  $("#mic-button").disabled = true;
  let stream;
  try { stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true }, video: false }); }
  catch (error) { updatePendingUI(); if (captureID === captureRequest) toast(error.name === "NotAllowedError" ? "还没有获得麦克风权限。可以在浏览器地址栏允许录音，或直接打字。" : "没有找到可用的麦克风，请检查设备连接。"); return; }
  if (captureID !== captureRequest) { stream.getTracks().forEach((track) => track.stop()); updatePendingUI(); return; }
  const task = { stream, started: Date.now(), chunks: [], cancelled: false, transcript: "", ending: false };
  recording = task;
  $("#recording-banner").hidden = false;
  $("#recording-label").textContent = config.serverASR ? "正在录音" : "正在聆听 · 浏览器识别";
  $("#recording-time").textContent = "00:00";
  $("#mic-button").classList.add("recording"); $("#mic-button").innerHTML = icon("stop");
  $("#mic-button").setAttribute("aria-label", "结束录音"); $("#mic-button").title = "结束录音";
  $("#input-hint").textContent = "说完后，点一下结束";
  recordingTimer = setInterval(() => {
    if (recording !== task) return;
    const seconds = (Date.now() - task.started) / 1000;
    $("#recording-time").textContent = formatTime(seconds);
    if (seconds >= 60) finishRecording();
  }, 200);
  if (config.serverASR) {
    const mimeType = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"].find((type) => MediaRecorder.isTypeSupported(type));
    try {
      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : {}); task.recorder = recorder;
      recorder.ondataavailable = (event) => { if (event.data.size && !task.cancelled) task.chunks.push(event.data); };
      recorder.onstop = () => { stream.getTracks().forEach((track) => track.stop()); if (recording === task) recording = null; if (!task.cancelled) transcribeRecording(task); };
      recorder.onerror = () => { if (recording === task) cancelRecording(); toast("录音中断了，请重新试一次。"); };
      recorder.start();
    } catch { cancelRecording(); toast("当前设备没能开始录音，文字聊天仍然可用。"); }
  } else {
    // SpeechRecognition owns its own capture; release the permission-check stream.
    stream.getTracks().forEach((track) => track.stop());
    const recognition = new SpeechRecognition(); task.recognition = recognition;
    recognition.lang = "zh-CN"; recognition.continuous = true; recognition.interimResults = true;
    recognition.onresult = (event) => {
      if (task.cancelled) return;
      task.transcript = [...event.results].map((result) => result[0].transcript).join("");
      $("#recording-label").textContent = task.transcript ? `听到：${task.transcript.slice(0, 28)}` : "正在聆听";
    };
    recognition.onerror = (event) => {
      if (task.cancelled || event.error === "aborted") return;
      task.failed = true;
      toast(event.error === "network" ? "浏览器的识别服务没有连上。可以检查网络，或先用文字输入。" : event.error === "not-allowed" ? "麦克风权限没有开启，可以先打字。" : "没听清这段话，可以重新录一遍。");
    };
    recognition.onend = () => {
      if (task.cancelled) return;
      if (recording === task) recording = null;
      clearRecordingUI();
      if (task.transcript.trim()) setTranscript(task.transcript);
      else if (!task.failed) toast("没有识别到清楚的话，可以重新录音或手动输入。");
    };
    try { recognition.start(); }
    catch { cancelRecording(); toast("浏览器没能开始语音识别，可以先直接打字。"); }
  }
  updatePendingUI();
}

function finishRecording() {
  const task = recording;
  if (!task || task.ending) return;
  task.ending = true;
  clearInterval(recordingTimer);
  if (task.recognition) {
    $("#recording-label").textContent = "识别中…";
    task.recognition.stop();
    // Some browser engines never fire an end event after a service failure.
    setTimeout(() => { if (recording === task) { const text = task.transcript; cancelRecording(); if (text) setTranscript(text); else toast("语音识别没有回应，可以重新录音或打字。"); } }, 5000);
  } else {
    $("#mic-button").disabled = true;
    if (task.recorder.state === "recording") task.recorder.stop();
    task.stream.getTracks().forEach((track) => track.stop());
  }
}

async function transcribeRecording(task) {
  if (task.cancelled) return;
  const controller = new AbortController(); transcribing = controller;
  $("#recording-label").textContent = "识别中…"; updatePendingUI();
  const type = task.recorder.mimeType;
  const ext = type.includes("mp4") ? "mp4" : type.includes("ogg") ? "ogg" : "webm";
  const data = new FormData(); data.append("audio", new Blob(task.chunks, { type }), `recording.${ext}`);
  try {
    const result = await api("/api/transcribe", { method: "POST", body: data, signal: controller.signal });
    if (transcribing === controller) setTranscript(result.text);
  } catch (error) { if (error.name !== "AbortError") toast(error); }
  finally { task.chunks = []; if (transcribing === controller) transcribing = null; clearRecordingUI(); }
}

function setTranscript(text) {
  $("#message-input").value = [...text.trim()].slice(0, 1000).join("");
  $("#input-hint").textContent = "确认文字后，点发送";
  $("#message-input").focus(); toast("已经听到了。你可以先改一下文字，再发送。");
}

function bindEvents() {
  $$("[data-view]").forEach((el) => el.addEventListener("click", () => switchView(el.dataset.view)));
  $("#new-session").addEventListener("click", newSession); $("#chat-new").addEventListener("click", newSession);
  $("#chat-form").addEventListener("submit", (event) => { event.preventDefault(); sendMessage($("#message-input").value).catch(toast); });
  $("#message-input").addEventListener("keydown", (event) => { if (event.key === "Enter" && !event.shiftKey && !event.isComposing) { event.preventDefault(); sendMessage(event.currentTarget.value).catch(toast); } });
  $$("[data-prompt]").forEach((el) => el.addEventListener("click", () => { switchView("home"); sendMessage(el.dataset.prompt).catch(toast); }));
  $("#cancel-chat").addEventListener("click", () => { cancelPending(); toast("已经停下来了，想好了再说也没关系。"); });
  $("#quick-music").addEventListener("click", () => switchView("music"));
  $("#voice-toggle").addEventListener("click", () => saveSettings({ ...state.settings, voice: !state.settings.voice }, false).catch(toast));
  $("#choose-companion").addEventListener("click", () => switchView("settings"));
  $("#add-companion").addEventListener("click", () => openCompanion());
  $("#edit-companion").addEventListener("click", () => openCompanion(currentCompanion()));
  $("#apply-anon-template").addEventListener("click", () => chooseCompanion("template-anon").catch(toast));
  $("#companion-image").addEventListener("change", (event) => uploadCompanionImage(event.target.files[0]).catch(toast));
  $("#companion-image-remove").addEventListener("click", () => { cancelCompanionImageUpload(); companionImageURL = ""; $("#companion-image").value = ""; $("#companion-image-status").textContent = "保存后恢复默认形象。"; renderCompanionImage(); });
  $("#companion-dialog").addEventListener("close", cancelCompanionImageUpload);
  $("#companion-theme").addEventListener("change", () => { if ($("#companion-theme").value === "anon" && $("#companion-voice").value === "default") $("#companion-voice").value = "anon"; });
  $("#settings-companion-voice").addEventListener("change", () => {
    stopReplyAudio(); cleanSpeechCache(); renderVoiceSettings(); updateVoiceNotes();
    $("#companion-voice-status").textContent = previewVoice() === savedVoice() ? `已保存：${voiceName()}。` : `尚未保存，聊天仍使用${voiceName()}；可以先试听当前选择。`;
  });
  $("#save-companion-voice").addEventListener("click", () => saveCompanionVoice().catch(toast));
  $("#speech-language").addEventListener("change", () => {
    stopReplyAudio(); cleanSpeechCache();
    const samples = ["你好呀，今天也想陪你聊聊天。", "你好呀，今日都想陪你倾下偈。"];
    if (samples.includes($("#voice-preview-text").value)) $("#voice-preview-text").value = samples[$("#speech-language").value === "yue" ? 1 : 0];
    updateVoiceNotes();
  });
  $("#companion-select").addEventListener("change", (event) => { const id = event.currentTarget.value; chooseCompanion(id).catch((error) => { renderCompanion(); toast(error); }); });
  $("#companion-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (companionImageRequest) { toast("参考图还在保存，请稍等一下。"); return; }
    const id = $("#companion-id").value;
    const body = { name: $("#companion-name").value.trim(), identity: $("#companion-identity").value.trim(), personality: $("#companion-personality").value.trim(), speakingStyle: $("#companion-speaking-style").value.trim(), greeting: $("#companion-greeting").value.trim(), theme: $("#companion-theme").value, voice: $("#companion-voice").value, imageUrl: companionImageURL };
    const button = event.submitter; if (button) button.disabled = true;
    try {
      const companion = await api(id ? `/api/companions/${id}` : "/api/companions", { method: id ? "PUT" : "POST", body });
      if (id) state.companions = state.companions.map((c) => c.id === id ? companion : c); else state.companions.push(companion);
      $("#companion-dialog").close();
      renderHistory(); renderMessages();
      if (!id) { toast("伴侣已保存，正在开启对话。"); await chooseCompanion(companion.id); }
      else toast("伴侣设定已保存，会用于下一条新回复。");
    } catch (error) { toast(error); renderCompanion(); }
    finally { if (button) button.disabled = false; }
  });
  $("#voice-preview").addEventListener("click", () => speakText($("#voice-preview-text").value, Number($("#volume").value), $("#speech-language").value, previewVoice()).catch(toast));
  $("#voice-preview-stop").addEventListener("click", stopReplyAudio);
  $("#stop-voice").addEventListener("click", stopReplyAudio);
  $("#mic-button").addEventListener("click", () => toggleRecording().catch(toast));
  $("#cancel-recording").addEventListener("click", () => { cancelRecording(); $("#input-hint").textContent = ""; toast("这次录音已取消，没有发送。"); });
  $("#player-toggle").addEventListener("click", async () => { if (!currentSong) return; if (songAudio.paused) { try { await songAudio.play(); } catch { toast("声音没能开始播放，请重新选择歌曲。"); } } else songAudio.pause(); updatePlayer(); updateCharacter(); });
  $("#player-close").addEventListener("click", stopPlayback);
  $("#player-progress").addEventListener("input", (event) => { if (currentSong) songAudio.currentTime = Number(event.currentTarget.value); updatePlayer(); });
  songAudio.addEventListener("timeupdate", updatePlayer);
  songAudio.addEventListener("play", () => { updatePlayer(); updateCharacter(); });
  songAudio.addEventListener("pause", () => { updatePlayer(); updateCharacter(); });
  songAudio.addEventListener("ended", stopPlayback);
  songAudio.addEventListener("error", () => { if (currentSong) { stopPlayback(); toast("这首小曲没有加载成功，可以换一首或重新播放。"); } });
  replyAudio.addEventListener("playing", () => { speaking = true; updateCharacter(); });
  replyAudio.addEventListener("ended", () => { speaking = false; updateCharacter(); });
  replyAudio.addEventListener("pause", () => { speaking = false; updateCharacter(); });
  replyAudio.addEventListener("error", () => { if (replyAudio.getAttribute("src")) { speaking = false; updateCharacter(); toast("声音文件暂时无法播放，文字回复仍然保留。"); } });
  $("#add-memory").addEventListener("click", () => openMemory());
  $("#memory-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const content = $("#memory-content").value.trim(); if (!content) return;
    const id = $("#memory-id").value;
    const button = event.submitter; if (button) button.disabled = true;
    try {
      const memory = await api(id ? `/api/memories/${id}` : "/api/memories", { method: id ? "PUT" : "POST", body: { content, category: $("#memory-category").value } });
      if (id) state.memories = state.memories.map((m) => m.id === id ? memory : m); else state.memories.push(memory);
      $("#memory-dialog").close(); renderMemories(); toast("这件小事已经放进记忆瓶啦。");
    } catch (error) { toast(error); }
    finally { if (button) button.disabled = false; }
  });
  $$("[data-close-dialog]").forEach((el) => el.addEventListener("click", () => el.closest("dialog").close()));
  $("#confirm-cancel").addEventListener("click", () => finishConfirm(false));
  $("#confirm-accept").addEventListener("click", () => finishConfirm(true));
  $("#confirm-dialog").addEventListener("cancel", (event) => { event.preventDefault(); finishConfirm(false); });
  $("#settings-form").addEventListener("submit", (event) => { event.preventDefault(); saveSettings({ nickname: $("#nickname").value.trim(), voice: $("#setting-voice").checked, volume: Number($("#volume").value), speechLanguage: $("#speech-language").value }).catch(toast); });
  $("#volume").addEventListener("input", (event) => { $("#volume-value").textContent = `${event.target.value}%`; songAudio.volume = Number(event.target.value) / 100; replyAudio.volume = Number(event.target.value) / 100; });
  $("#clear-history").addEventListener("click", async () => {
    if (!await confirmAction("清空所有聊天记录？", "所有对话都会删除。记忆瓶与偏好设置会保留。")) return;
    cancelPending(); cancelRecording(); stopPlayback(); cleanSpeechCache();
    try { await api("/api/sessions", { method: "DELETE" }); state.sessions = []; await createSession(); renderHistory(); renderMessages(); toast("聊天记录已清空，记忆瓶仍然保留。"); } catch (error) { toast(error); }
  });
  $("#clear-memories").addEventListener("click", async () => {
    if (!await confirmAction("清空整个记忆瓶？", "所有主动保存的记忆都会删除，聊天记录和设置里的昵称会保留。建议新建对话以使用清空后的记忆。")) return;
    try { await api("/api/memories", { method: "DELETE" }); state.memories = []; renderMemories(); toast("记忆瓶已清空，聊天记录没有删除。"); } catch (error) { toast(error); }
  });
  $("#mode-badge").addEventListener("click", () => $("#mode-dialog").showModal());
  $("#mode-got-it").addEventListener("click", () => $("#mode-dialog").close());
  document.addEventListener("visibilitychange", () => { if (document.hidden && (recording || transcribing)) cancelRecording(); });
  window.addEventListener("pagehide", () => { cancelPending(); cancelRecording(); stopPlayback(); cleanSpeechCache(); });
  $("#character-sheet").addEventListener("error", () => { $("#whale-character").hidden = true; $("#character-bubble").textContent = "伴侣的照片还没加载好，但我还在这里。"; });
  $("#portrait-touch")?.addEventListener("click", () => { touchedUntil = Date.now() + 2400; touchIndex++; updateCharacter(); setTimeout(updateCharacter, 2450); });
  $("#character-portrait").addEventListener("error", () => { $("#character-portrait").hidden = true; $("#character-bubble").textContent = "图片暂时没有加载好，我们先聊一会儿。"; });
  $("#whale-character").addEventListener("click", () => {
    touchIndex++; touchedUntil = Date.now() + 2400; updateCharacter();
    setTimeout(updateCharacter, 2450);
  });
}

async function init() {
  bindEvents();
  const weekday = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
  const today = new Date(); $("#today").textContent = `${String(today.getMonth() + 1).padStart(2, "0")} / ${String(today.getDate()).padStart(2, "0")}  ${weekday[today.getDay()]}`;
  try {
    [config, state] = await Promise.all([api("/api/config"), api("/api/state")]);
    const live = config.chatMode === "live";
    $("#mode-badge").classList.toggle("live", live);
    $("#mode-badge").innerHTML = `<span></span>${live ? "AI 已连接" : "演示模式"}${icon("info")}`;
    $("#chat-disclaimer").textContent = live ? "回复由 AI 生成。重要的信息，请保留自己的判断。" : "当前为本地演示回复，用于体验互动流程。";
    $("#connection-tag").textContent = live ? "AI 已连接" : "演示模式";
    $("#connection-copy").textContent = `${live ? "AI 对话" : "本地演示回复"} · ${config.serverASR ? "录音识别服务" : "浏览器识别"} · ${config.serverTTS ? "角色语音服务" : "设备声音"} · 合成哼唱`;
    $("#voice-service-note").textContent = config.ttsProvider === "gpt-sovits" ? "使用 GPT-SoVITS 本地推理；每次试听和朗读都会实时合成，首次加载模型可能较慢。" : config.serverTTS ? "已连接语音合成服务，使用后端配置的音色。" : "使用这台设备的中文朗读声音，音色取决于本机可用声音。";
    $("#voice-preview-note").textContent = config.ttsProvider === "gpt-sovits" ? "每次点击都会调用 GPT-SoVITS 实时合成；首次推理可能需要等待。使用上方音量，无需开启自动朗读。" : "输入一句话，使用当前语音服务播放。使用上方音量，无需开启自动朗读。";
    $("#mode-copy").textContent = live ? "当前已连接 AI 对话服务，聊天会参考本次对话和你主动保存的记忆。" : "当前聊天由本地规则生成，适合体验角色、会话和记忆流程。配置 AI 服务后，可以开启真实对话。";
    let homeVersionSeen = false;
    try {
      currentSessionID = localStorage.getItem("whale.currentSession") || "";
      homeVersionSeen = localStorage.getItem("whale.homeVersion2") === "1";
      localStorage.removeItem("whale.anonTemplateSeen");
    } catch { /* Optional storage. */ }
    if (!homeVersionSeen || !state.sessions.some((item) => item.id === currentSessionID)) {
      currentSessionID = state.sessions.find((item) => item.companionId === state.activeCompanionId)?.id || "";
    }
    if (!currentSessionID) await createSession(state.activeCompanionId || "default-whale");
    try { localStorage.setItem("whale.homeVersion2", "1"); } catch { /* Optional storage. */ }
    persistSessionID(); renderHistory(); renderMessages(); renderMusic(); renderMemories(); renderSettings(); updateCharacter();
  } catch (error) {
    toast(error); $("#messages").append(node("p", "empty-error", "鲸伴暂时没有连上本地服务。启动后刷新页面，就能见面啦。"));
    $("#send-button").disabled = true; $("#mic-button").disabled = true;
  }
}

init();
