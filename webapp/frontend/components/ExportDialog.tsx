"use client";
import { useState } from "react";
import { api } from "@/lib/api";

interface Props { docId: string; onClose: () => void; }

export function ExportDialog({ docId, onClose }: Props) {
  const [format, setFormat] = useState("markdown");
  const [theme, setTheme] = useState("light");
  const [loading, setLoading] = useState(false);

  async function handleExport() {
    setLoading(true);
    try {
      const res = await api.exportDocument(docId, format, theme);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = res.headers.get("Content-Disposition")?.split("filename=")[1] ?? `export.${format}`;
      a.click();
      URL.revokeObjectURL(url);
      onClose();
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-2xl p-6 w-80 space-y-4 shadow-xl">
        <h2 className="text-lg font-semibold">Export Document</h2>
        <label className="flex flex-col gap-1 text-sm">
          Format
          <select value={format} onChange={e => setFormat(e.target.value)} className="border rounded px-2 py-1">
            <option value="markdown">Markdown</option>
            <option value="txt">Plain Text</option>
            <option value="pdf">PDF</option>
            <option value="docx">Word (DOCX)</option>
            <option value="html">HTML</option>
          </select>
        </label>
        {format === "pdf" && (
          <label className="flex flex-col gap-1 text-sm">
            Theme
            <select value={theme} onChange={e => setTheme(e.target.value)} className="border rounded px-2 py-1">
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </select>
          </label>
        )}
        <div className="flex gap-2 justify-end">
          <button onClick={onClose} className="px-4 py-2 border rounded-lg text-sm">Cancel</button>
          <button onClick={handleExport} disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm disabled:opacity-50">
            {loading ? "Exporting…" : "Export"}
          </button>
        </div>
      </div>
    </div>
  );
}
