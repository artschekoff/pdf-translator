"use client";
import { use, useEffect, useState, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import dynamic from "next/dynamic";
import { api, type Document, type BlockData } from "@/lib/api";
import { AiPanel } from "@/components/AiPanel";
import { ExportDialog } from "@/components/ExportDialog";

const BlockEditor = dynamic(
  () => import("@/components/BlockEditor").then(m => m.BlockEditor),
  { ssr: false }
);

export default function EditorPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [doc, setDoc] = useState<Document | null>(null);
  const [selectedBlock, setSelectedBlock] = useState<string>("");
  const [showExport, setShowExport] = useState(false);
  const [saving, setSaving] = useState(false);
  const pageIdRef = useRef<string>("");

  useEffect(() => {
    api.getDocument(id).then(d => {
      setDoc(d);
      if (d.pages?.[0]) pageIdRef.current = d.pages[0].id;
    }).catch(() => router.push("/documents"));
  }, [id, router]);

  const handleSave = useCallback(async (blocks: BlockData[]) => {
    if (!pageIdRef.current) return;
    setSaving(true);
    try { await api.saveBlocks(id, pageIdRef.current, blocks); }
    finally { setSaving(false); }
  }, [id]);

  if (!doc) return <div className="p-8 text-center">Loading…</div>;

  const initialBlocks = doc.pages?.[0]?.blocks ?? [];

  return (
    <div className="flex h-screen">
      <div className="flex-1 flex flex-col overflow-hidden">
        <div className="border-b px-4 py-2 flex items-center justify-between bg-white">
          <div className="flex items-center gap-4">
            <button onClick={() => router.push("/documents")} className="text-gray-500 hover:text-gray-700">← Back</button>
            <span className="font-semibold">{doc.title}</span>
            {saving && <span className="text-xs text-gray-400">Saving…</span>}
          </div>
          <button onClick={() => setShowExport(true)}
            className="px-3 py-1 border rounded-lg text-sm hover:bg-gray-50">Export</button>
        </div>
        <div className="flex-1 overflow-y-auto p-6">
          <BlockEditor initialBlocks={initialBlocks} onSave={handleSave} onBlockSelect={setSelectedBlock} />
        </div>
      </div>
      <div className="w-80 border-l bg-gray-50 overflow-hidden flex flex-col">
        <AiPanel blockText={selectedBlock} />
      </div>
      {showExport && <ExportDialog docId={id} onClose={() => setShowExport(false)} />}
    </div>
  );
}
