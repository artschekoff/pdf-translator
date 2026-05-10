"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, type OcrSettings } from "@/lib/api";
import { OcrSettingsForm } from "@/components/OcrSettingsForm";
import { ProcessingProgress } from "@/components/ProcessingProgress";

export default function UploadPage() {
  const router = useRouter();
  const [file, setFile] = useState<File | null>(null);
  const [settings, setSettings] = useState<OcrSettings>({
    engine: "rapidocr", lang: ["en"], do_ocr: true,
    do_table_structure: true, table_mode: "fast",
    generate_picture_images: true, images_scale: 2.0, document_timeout: 300,
  });
  const [docId, setDocId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [balance, setBalance] = useState<number | null>(null);

  useEffect(() => {
    api.getBalance().then(r => setBalance(r.pages)).catch(() => {});
  }, []);

  async function handleUpload() {
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      const doc = await api.uploadDocument(file, settings);
      await api.startOcr(doc.id);
      setDocId(doc.id);
    } catch (e) {
      setError(String(e));
      setUploading(false);
    }
  }

  if (docId) {
    return (
      <div className="max-w-xl mx-auto p-8 text-center">
        <h1 className="text-xl font-bold mb-4">Processing…</h1>
        <ProcessingProgress docId={docId} onComplete={() => router.push(`/editor/${docId}`)} />
      </div>
    );
  }

  return (
    <div className="max-w-xl mx-auto p-8 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Upload PDF</h1>
        {balance !== null && (
          <a href="/billing" className="text-sm text-blue-600 hover:underline">
            {balance} pages remaining
          </a>
        )}
      </div>
      {balance === 0 && (
        <div className="bg-amber-50 border border-amber-200 rounded-lg p-4 text-sm">
          You have no pages left. <a href="/billing" className="text-blue-600 underline">Buy more pages</a> to process PDFs.
        </div>
      )}
      {error && <div className="text-red-500 text-sm">{error}</div>}
      <input type="file" accept=".pdf" onChange={e => setFile(e.target.files?.[0] ?? null)}
        className="block w-full border rounded-lg p-2" />
      <OcrSettingsForm onChange={setSettings} />
      <button onClick={handleUpload} disabled={!file || uploading}
        className="w-full bg-blue-600 text-white py-2 rounded-lg hover:bg-blue-700 disabled:opacity-50">
        {uploading ? "Uploading…" : "Upload & Process"}
      </button>
    </div>
  );
}
