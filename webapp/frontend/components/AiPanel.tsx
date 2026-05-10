"use client";
import { useState, useRef, useEffect, useCallback } from "react";
import { tokenStore } from "@/lib/auth";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api";

type Mode = "ask" | "edit";
type Provider = "claude" | "openai";

interface Message {
  role: "user" | "ai";
  content: string;
}

const CLAUDE_MODELS = [
  "claude-sonnet-4-6",
  "claude-opus-4-5",
  "claude-haiku-3-5",
];

const OPENAI_MODELS = [
  "gpt-4o",
  "gpt-4o-mini",
  "gpt-4-turbo",
];

interface Props {
  blockText: string;
}

export function AiPanel({ blockText }: Props) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [mode, setMode] = useState<Mode>("ask");
  const [provider, setProvider] = useState<Provider>("claude");
  const [model, setModel] = useState(CLAUDE_MODELS[0]);
  const [prompt, setPrompt] = useState("");
  const [loading, setLoading] = useState(false);
  const [streamText, setStreamText] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, streamText]);

  // Auto-resize textarea
  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return;
    ta.style.height = "auto";
    ta.style.height = Math.min(ta.scrollHeight, 150) + "px";
  }, [prompt]);

  // Switch model list when provider changes
  useEffect(() => {
    setModel(provider === "claude" ? CLAUDE_MODELS[0] : OPENAI_MODELS[0]);
  }, [provider]);

  const handleSubmit = useCallback(async () => {
    const instruction = prompt.trim();
    if (!instruction || loading) return;

    setPrompt("");
    setLoading(true);
    setStreamText("");

    const userMsg: Message = { role: "user", content: instruction };
    setMessages(prev => [...prev, userMsg]);

    try {
      abortRef.current = new AbortController();
      const res = await fetch(`${BASE}/ai/block`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${tokenStore.get()}`,
        },
        body: JSON.stringify({
          block_text: blockText,
          instruction,
          mode,
          provider,
          model,
        }),
        signal: abortRef.current.signal,
      });

      if (!res.ok) {
        const errText = await res.text();
        setMessages(prev => [...prev, { role: "ai", content: `Error: ${errText}` }]);
        return;
      }

      const contentType = res.headers.get("content-type") ?? "";

      // Streaming SSE
      if (contentType.includes("text/event-stream") && res.body) {
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let accumulated = "";
        let buffer = "";

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() ?? "";
          for (const line of lines) {
            if (line.startsWith("data: ")) {
              const data = line.slice(6).trim();
              if (data === "[DONE]") break;
              try {
                const parsed = JSON.parse(data);
                const chunk = parsed.text ?? parsed.content ?? parsed.delta ?? "";
                accumulated += chunk;
                setStreamText(accumulated);
              } catch {
                // plain text chunk
                accumulated += data;
                setStreamText(accumulated);
              }
            }
          }
        }

        setMessages(prev => [...prev, { role: "ai", content: accumulated }]);
        setStreamText("");
      } else {
        // Non-streaming JSON
        const json = await res.json();
        const content = json.content ?? json.text ?? JSON.stringify(json);
        setMessages(prev => [...prev, { role: "ai", content }]);
      }
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        setMessages(prev => [...prev, { role: "ai", content: `Error: ${String(err)}` }]);
      }
    } finally {
      setLoading(false);
      abortRef.current = null;
    }
  }, [prompt, loading, blockText, mode, provider, model]);

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
      e.preventDefault();
      handleSubmit();
    }
  }

  function clearHistory() {
    setMessages([]);
    setStreamText("");
  }

  const modelOptions = provider === "claude" ? CLAUDE_MODELS : OPENAI_MODELS;

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="px-3 py-2 border-b bg-white flex items-center justify-between">
        <span className="font-semibold text-sm">AI Assistant</span>
        <button onClick={clearHistory} className="text-xs text-gray-400 hover:text-gray-600">
          Clear
        </button>
      </div>

      {/* Controls */}
      <div className="px-3 py-2 border-b bg-white space-y-1">
        <div className="flex gap-2 text-xs">
          <label className="flex flex-col gap-0.5 flex-1">
            Mode
            <select value={mode} onChange={e => setMode(e.target.value as Mode)}
              className="border rounded px-1 py-0.5 text-xs">
              <option value="ask">Ask</option>
              <option value="edit">Edit</option>
            </select>
          </label>
          <label className="flex flex-col gap-0.5 flex-1">
            Provider
            <select value={provider} onChange={e => setProvider(e.target.value as Provider)}
              className="border rounded px-1 py-0.5 text-xs">
              <option value="claude">Claude</option>
              <option value="openai">OpenAI</option>
            </select>
          </label>
        </div>
        <label className="flex flex-col gap-0.5 text-xs">
          Model
          <select value={model} onChange={e => setModel(e.target.value)}
            className="border rounded px-1 py-0.5 text-xs">
            {modelOptions.map(m => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        </label>
        {blockText && (
          <div className="text-xs text-gray-400 truncate" title={blockText}>
            Context: {blockText.slice(0, 60)}{blockText.length > 60 ? "…" : ""}
          </div>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-3 space-y-3 text-sm">
        {messages.length === 0 && !loading && (
          <p className="text-gray-400 text-xs text-center pt-4">
            Select a block, then ask a question or request an edit.
          </p>
        )}
        {messages.map((msg, i) => (
          <div key={i} className={`rounded-lg p-2 text-sm whitespace-pre-wrap ${
            msg.role === "user"
              ? "bg-blue-50 text-blue-900 self-end"
              : "bg-gray-100 text-gray-800"
          }`}>
            <div className="text-xs font-semibold mb-1 opacity-60">
              {msg.role === "user" ? "You" : "AI"}
            </div>
            {msg.content}
          </div>
        ))}
        {loading && streamText && (
          <div className="rounded-lg p-2 bg-gray-100 text-gray-800 text-sm whitespace-pre-wrap">
            <div className="text-xs font-semibold mb-1 opacity-60">AI</div>
            {streamText}
          </div>
        )}
        {loading && !streamText && (
          <div className="text-xs text-gray-400 animate-pulse">AI is thinking…</div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t p-2 bg-white">
        <textarea
          ref={textareaRef}
          value={prompt}
          onChange={e => setPrompt(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={mode === "ask" ? "Ask about this block… (Cmd+Enter)" : "Describe how to edit… (Cmd+Enter)"}
          rows={2}
          className="w-full border rounded-lg px-3 py-2 text-sm resize-none focus:outline-none focus:ring-1 focus:ring-blue-400"
          disabled={loading}
        />
        <button
          onClick={handleSubmit}
          disabled={loading || !prompt.trim()}
          className="mt-1 w-full bg-blue-600 text-white py-1.5 rounded-lg text-sm hover:bg-blue-700 disabled:opacity-50"
        >
          {loading ? "…" : mode === "ask" ? "Ask" : "Edit"}
        </button>
      </div>
    </div>
  );
}
