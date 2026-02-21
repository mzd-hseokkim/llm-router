"use client";

import { useState, useRef, useEffect, useCallback } from "react";

const PRESET_MODELS = [
  "anthropic/claude-sonnet-4-6",
  "anthropic/claude-haiku-4-5-20251001",
  "anthropic/claude-3-5-sonnet-20241022",
  "anthropic/claude-3-5-haiku-20241022",
  "openai/gpt-4o",
  "openai/gpt-4o-mini",
  "openai/gpt-3.5-turbo",
  "gemini/gemini-2.0-flash",
  "gemini/gemini-1.5-pro",
];

const STORAGE_KEY = "playground_conversations";

type Message = { role: "user" | "assistant"; content: string };

type Conversation = {
  id: string;
  title: string;
  keyId: string;
  model: string;
  systemPrompt: string;
  messages: Message[];
  createdAt: string;
};

type VirtualKey = {
  id: string;
  name: string;
  key_prefix: string;
  is_active: boolean;
};

function genId() {
  return Math.random().toString(36).slice(2) + Date.now().toString(36);
}

function loadConversations(): Conversation[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || "[]");
  } catch {
    return [];
  }
}

function saveConversations(convs: Conversation[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(convs));
}

function MessageBubble({ msg }: { msg: Message }) {
  const isUser = msg.role === "user";
  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[75%] rounded-2xl px-4 py-3 text-sm whitespace-pre-wrap break-words leading-relaxed ${
          isUser
            ? "bg-slate-800 text-white rounded-br-sm"
            : "bg-white border border-slate-200 text-slate-800 rounded-bl-sm shadow-sm"
        }`}
      >
        {msg.content || <span className="animate-pulse text-slate-400">▋</span>}
      </div>
    </div>
  );
}

export default function PlaygroundPage() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [currentId, setCurrentId] = useState<string | null>(null);
  const [model, setModel] = useState("anthropic/claude-sonnet-4-6");
  const [keyId, setKeyId] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState<number | "">("");
  const [error, setError] = useState("");
  const [keys, setKeys] = useState<VirtualKey[]>([]);
  const [keysLoading, setKeysLoading] = useState(true);

  const bottomRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Load conversations from localStorage on mount
  useEffect(() => {
    setConversations(loadConversations());
  }, []);

  // Fetch virtual keys
  useEffect(() => {
    fetch("/api/admin/keys?limit=200")
      .then((r) => r.json())
      .then((data) => {
        const active = ((data.data ?? data) as VirtualKey[]).filter(
          (k) => k.is_active
        );
        setKeys(active);
        if (active.length > 0 && !keyId) setKeyId(active[0].id);
      })
      .catch(() => {})
      .finally(() => setKeysLoading(false));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const persistConversations = useCallback((convs: Conversation[]) => {
    setConversations(convs);
    saveConversations(convs);
  }, []);

  function newChat() {
    abortRef.current?.abort();
    setCurrentId(null);
    setMessages([]);
    setError("");
    setInput("");
    textareaRef.current?.focus();
  }

  function loadConversation(conv: Conversation) {
    abortRef.current?.abort();
    setCurrentId(conv.id);
    setModel(conv.model);
    setKeyId(conv.keyId);
    setSystemPrompt(conv.systemPrompt);
    setMessages(conv.messages);
    setError("");
    setInput("");
  }

  function deleteConversation(id: string, e: React.MouseEvent) {
    e.stopPropagation();
    const next = conversations.filter((c) => c.id !== id);
    persistConversations(next);
    if (currentId === id) newChat();
  }

  async function send() {
    const text = input.trim();
    if (!text || !keyId || isStreaming) return;

    setError("");
    const userMsg: Message = { role: "user", content: text };
    const nextMessages = [...messages, userMsg];
    setMessages([...nextMessages, { role: "assistant", content: "" }]);
    setInput("");
    setIsStreaming(true);

    abortRef.current = new AbortController();

    // Determine conversation id (create new if needed)
    let convId = currentId;
    if (!convId) {
      convId = genId();
      setCurrentId(convId);
    }

    try {
      const body: Record<string, unknown> = {
        key_id: keyId,
        model,
        messages: [
          ...(systemPrompt ? [{ role: "system", content: systemPrompt }] : []),
          ...nextMessages.map((m) => ({ role: m.role, content: m.content })),
        ],
        stream: true,
        temperature,
      };
      if (maxTokens !== "") body.max_tokens = maxTokens;

      const res = await fetch("/api/admin/playground", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
        signal: abortRef.current.signal,
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err?.error?.message ?? `HTTP ${res.status}`);
      }

      const reader = res.body!.getReader();
      const decoder = new TextDecoder();
      let buf = "";
      let assistantContent = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const lines = buf.split("\n");
        buf = lines.pop()!;

        for (const line of lines) {
          if (!line.startsWith("data: ")) continue;
          const data = line.slice(6).trim();
          if (data === "[DONE]") break;
          try {
            const chunk = JSON.parse(data);
            const delta = chunk.choices?.[0]?.delta?.content ?? "";
            if (delta) {
              assistantContent += delta;
              setMessages((prev) => {
                const next = [...prev];
                const last = next[next.length - 1];
                next[next.length - 1] = { ...last, content: last.content + delta };
                return next;
              });
            }
          } catch {
            // malformed chunk — skip
          }
        }
      }

      // Persist the completed conversation
      const finalMessages: Message[] = [
        ...nextMessages,
        { role: "assistant", content: assistantContent },
      ];
      setConversations((prev) => {
        const exists = prev.find((c) => c.id === convId);
        let next: Conversation[];
        if (exists) {
          next = prev.map((c) =>
            c.id === convId ? { ...c, messages: finalMessages } : c
          );
        } else {
          const title = text.length > 50 ? text.slice(0, 47) + "…" : text;
          const newConv: Conversation = {
            id: convId!,
            title,
            keyId,
            model,
            systemPrompt,
            messages: finalMessages,
            createdAt: new Date().toISOString(),
          };
          next = [newConv, ...prev];
        }
        saveConversations(next);
        return next;
      });
    } catch (err: unknown) {
      if ((err as Error)?.name === "AbortError") {
        // user stopped — leave partial message
      } else {
        const msg = (err as Error)?.message ?? "Unknown error";
        setError(msg);
        setMessages((prev) => prev.slice(0, -1));
      }
    } finally {
      setIsStreaming(false);
      abortRef.current = null;
      textareaRef.current?.focus();
    }
  }

  function stop() {
    abortRef.current?.abort();
  }

  const selectedKey = keys.find((k) => k.id === keyId);

  return (
    <div className="flex -m-8 h-screen overflow-hidden">
      {/* ── Left panel: conversation history ── */}
      <div className="w-60 shrink-0 bg-slate-900 text-slate-100 flex flex-col border-r border-slate-700">
        <div className="p-3 border-b border-slate-700">
          <button
            onClick={newChat}
            className="w-full py-2 px-3 rounded-lg bg-slate-700 hover:bg-slate-600 text-sm font-medium text-white transition-colors text-left"
          >
            + New Chat
          </button>
        </div>
        <div className="flex-1 overflow-y-auto py-2">
          {conversations.length === 0 && (
            <p className="text-xs text-slate-500 px-3 py-4 text-center">
              No conversations yet
            </p>
          )}
          {conversations.map((conv) => (
            <div
              key={conv.id}
              onClick={() => loadConversation(conv)}
              className={`group flex items-start gap-1 px-3 py-2 cursor-pointer rounded-md mx-1 ${
                conv.id === currentId
                  ? "bg-slate-700 text-white"
                  : "text-slate-300 hover:bg-slate-800"
              }`}
            >
              <span className="flex-1 text-xs leading-snug line-clamp-2 break-words">
                {conv.title}
              </span>
              <button
                onClick={(e) => deleteConversation(conv.id, e)}
                className="shrink-0 opacity-0 group-hover:opacity-100 text-slate-500 hover:text-red-400 transition-opacity text-xs mt-0.5"
                title="Delete"
              >
                ✕
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* ── Main area ── */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* ── Config bar ── */}
        <div className="shrink-0 border-b border-slate-200 bg-white px-6 py-3 flex flex-wrap items-end gap-4">
          <div className="flex flex-col gap-1 min-w-[200px] flex-1">
            <label className="text-xs font-medium text-slate-500">Model</label>
            <input
              list="playground-models"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm font-mono"
              placeholder="provider/model"
            />
            <datalist id="playground-models">
              {PRESET_MODELS.map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
          </div>

          <div className="flex flex-col gap-1 min-w-[180px] flex-1">
            <label className="text-xs font-medium text-slate-500">Virtual Key</label>
            {keysLoading ? (
              <div className="text-xs text-slate-400 py-2">Loading…</div>
            ) : keys.length === 0 ? (
              <div className="text-xs text-red-500 py-2">No active keys</div>
            ) : (
              <select
                value={keyId}
                onChange={(e) => setKeyId(e.target.value)}
                className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
              >
                {keys.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.name} ({k.key_prefix}…)
                  </option>
                ))}
              </select>
            )}
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium text-slate-500">
              Temperature <span className="text-slate-400">({temperature})</span>
            </label>
            <input
              type="range"
              min={0}
              max={1}
              step={0.1}
              value={temperature}
              onChange={(e) => setTemperature(Number(e.target.value))}
              className="w-28 accent-slate-700"
            />
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium text-slate-500">Max Tokens</label>
            <input
              type="number"
              min={1}
              value={maxTokens}
              onChange={(e) =>
                setMaxTokens(e.target.value ? Number(e.target.value) : "")
              }
              className="w-24 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
              placeholder="default"
            />
          </div>
        </div>

        {/* ── System prompt ── */}
        <div className="shrink-0 border-b border-slate-200 bg-slate-50 px-6 py-2">
          <input
            value={systemPrompt}
            onChange={(e) => setSystemPrompt(e.target.value)}
            className="w-full bg-transparent text-sm text-slate-600 placeholder-slate-400 outline-none"
            placeholder="System prompt (optional)…"
          />
        </div>

        {/* ── Messages ── */}
        <div className="flex-1 overflow-y-auto px-6 py-6 space-y-4 bg-slate-50">
          {messages.length === 0 && (
            <p className="text-center text-slate-400 text-sm mt-24">
              {selectedKey
                ? `Using key: ${selectedKey.name} (${selectedKey.key_prefix}…) — Start chatting.`
                : "Select a virtual key and start chatting."}
            </p>
          )}
          {messages.map((msg, i) => (
            <MessageBubble key={i} msg={msg} />
          ))}
          <div ref={bottomRef} />
        </div>

        {/* ── Error bar ── */}
        {error && (
          <div className="shrink-0 bg-red-50 border-t border-red-200 px-6 py-2 text-sm text-red-700 flex items-center justify-between">
            <span>{error}</span>
            <button
              onClick={() => setError("")}
              className="text-red-400 hover:text-red-600 ml-4"
            >
              ✕
            </button>
          </div>
        )}

        {/* ── Input ── */}
        <div className="shrink-0 border-t border-slate-200 bg-white px-6 py-4">
          <div className="flex gap-3 items-end">
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  send();
                }
              }}
              rows={3}
              className="flex-1 border border-slate-300 rounded-xl px-4 py-3 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-slate-400"
              placeholder="Message… (Enter to send, Shift+Enter for newline)"
            />
            {isStreaming ? (
              <button
                onClick={stop}
                className="px-5 py-2.5 rounded-xl text-sm font-medium bg-red-500 text-white hover:bg-red-600 transition-colors"
              >
                Stop
              </button>
            ) : (
              <button
                onClick={send}
                disabled={!input.trim() || !keyId}
                className="px-5 py-2.5 rounded-xl text-sm font-medium bg-slate-800 text-white hover:bg-slate-700 disabled:opacity-40 transition-colors"
              >
                Send
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
