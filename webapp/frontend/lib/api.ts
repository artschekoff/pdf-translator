import { tokenStore } from "./auth";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api";

export interface OcrSettings {
  engine: string;
  lang: string[];
  do_ocr: boolean;
  do_table_structure: boolean;
  table_mode: string;
  generate_picture_images: boolean;
  images_scale: number;
  document_timeout: number;
}

export interface BlockData {
  id: string;
  type: string;
  data: Record<string, unknown>;
  order: number;
}

export interface Page {
  id: string;
  document_id: string;
  page_number: number;
  blocks: BlockData[];
}

export interface Document {
  id: string;
  user_id: string;
  title: string;
  original_filename: string;
  status: "pending" | "processing" | "ready" | "error";
  error_message?: string;
  page_count: number;
  created_at: string;
  pages?: Page[];
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const token = tokenStore.get();
  const res = await fetch(BASE + path, {
    ...init,
    headers: {
      ...(init?.headers ?? {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
  if (res.status === 401) {
    tokenStore.clear();
    if (typeof window !== "undefined") window.location.href = "/login";
    throw new Error("Unauthorized");
  }
  if (!res.ok) throw new Error(await res.text());
  if (res.status === 204) return undefined as T;
  return res.json();
}

export interface BillingPlan {
  id: string;
  name: string;
  pages: number;
  price_cents: number;
  is_active: boolean;
}

export const api = {
  // Billing
  listPlans: () => req<BillingPlan[]>("/billing/plans"),
  getBalance: () => req<{ pages: number }>("/billing/balance"),
  createCheckout: (planId: string) =>
    req<{ url: string }>("/billing/checkout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan_id: planId }),
    }),
  syncPurchases: () =>
    req<{ credited: number; balance: number }>("/billing/sync", { method: "POST" }),

  listDocuments: () => req<Document[]>("/documents"),
  getDocument: (id: string) => req<Document>(`/documents/${id}`),
  deleteDocument: (id: string) => req<void>(`/documents/${id}`, { method: "DELETE" }),

  uploadDocument: (file: File, ocrSettings: OcrSettings) => {
    const form = new FormData();
    form.append("file", file);
    form.append("ocr_settings", JSON.stringify(ocrSettings));
    return req<Document>("/documents", { method: "POST", body: form });
  },

  startOcr: (id: string) => req<{ status: string }>(`/documents/${id}/ocr`, { method: "POST" }),

  saveBlocks: (docId: string, pageId: string, blocks: BlockData[]) =>
    req<{ saved: number }>(`/documents/${docId}/blocks`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ page_id: pageId, blocks }),
    }),

  exportDocument: (docId: string, format: string, theme = "light") =>
    fetch(`${BASE}/documents/${docId}/export`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${tokenStore.get()}`,
      },
      body: JSON.stringify({ format, theme }),
    }),
};
