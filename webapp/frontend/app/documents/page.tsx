"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, type Document } from "@/lib/api";
import { tokenStore } from "@/lib/auth";

export default function DocumentsPage() {
  const router = useRouter();
  const [docs, setDocs] = useState<Document[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!tokenStore.get()) { router.push("/login"); return; }
    api.listDocuments().then(setDocs).catch(() => router.push("/login")).finally(() => setLoading(false));
  }, [router]);

  async function handleDelete(id: string) {
    await api.deleteDocument(id);
    setDocs(d => d.filter(doc => doc.id !== id));
  }

  if (loading) return <div className="p-8 text-center">Loading…</div>;

  return (
    <div className="max-w-4xl mx-auto p-8">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">My Documents</h1>
        <button onClick={() => router.push("/documents/upload")}
          className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700">
          Upload PDF
        </button>
      </div>
      {docs.length === 0 ? (
        <p className="text-gray-500 text-center py-12">No documents yet. Upload a PDF to get started.</p>
      ) : (
        <div className="space-y-3">
          {docs.map(doc => (
            <div key={doc.id} className="border rounded-xl p-4 flex items-center justify-between hover:bg-gray-50">
              <div>
                <div className="font-medium">{doc.title}</div>
                <div className="text-sm text-gray-400">{doc.status} · {doc.page_count} pages</div>
              </div>
              <div className="flex gap-2">
                {doc.status === "ready" && (
                  <button onClick={() => router.push(`/editor/${doc.id}`)}
                    className="px-3 py-1 bg-blue-600 text-white rounded-lg text-sm">Open</button>
                )}
                <button onClick={() => handleDelete(doc.id)}
                  className="px-3 py-1 border rounded-lg text-sm text-red-500 hover:bg-red-50">Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
