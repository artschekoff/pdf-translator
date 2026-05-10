"use client";
import { useEffect, useState } from "react";
import { connectProgress } from "@/lib/ws";

interface Props { docId: string; onComplete: () => void; }

export function ProcessingProgress({ docId, onComplete }: Props) {
  const [stage, setStage] = useState("loading");
  const [percent, setPercent] = useState(0);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const disconnect = connectProgress(docId, (msg) => {
      setStage(msg.stage);
      if (msg.percent !== undefined) setPercent(msg.percent);
      if (msg.error) setError(msg.error);
      if (msg.stage === "complete") onComplete();
    });
    return disconnect;
  }, [docId, onComplete]);

  if (error) return <div className="text-red-500 p-4">Error: {error}</div>;

  return (
    <div className="p-4 space-y-2">
      <div className="text-sm text-gray-600 capitalize">{stage}…</div>
      <div className="w-full bg-gray-200 rounded-full h-2">
        <div className="bg-blue-600 h-2 rounded-full transition-all" style={{ width: `${percent}%` }} />
      </div>
      <div className="text-xs text-gray-400">{percent}%</div>
    </div>
  );
}
