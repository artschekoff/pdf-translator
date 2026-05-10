export function connectProgress(
  docId: string,
  onMessage: (msg: { stage: string; percent?: number; error?: string }) => void,
): () => void {
  const wsBase = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api")
    .replace(/^http/, "ws");
  const ws = new WebSocket(`${wsBase}/ws/${docId}/progress`);
  ws.onmessage = (e) => {
    try { onMessage(JSON.parse(e.data)); } catch {}
  };
  return () => ws.close();
}
