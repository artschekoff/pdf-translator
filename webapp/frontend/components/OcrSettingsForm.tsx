"use client";
import { useState } from "react";
import type { OcrSettings } from "@/lib/api";

const DEFAULT: OcrSettings = {
  engine: "rapidocr", lang: ["en"], do_ocr: true,
  do_table_structure: true, table_mode: "fast",
  generate_picture_images: true, images_scale: 2.0, document_timeout: 300,
};

export function OcrSettingsForm({ onChange }: { onChange: (s: OcrSettings) => void }) {
  const [s, setS] = useState<OcrSettings>(DEFAULT);

  function update(patch: Partial<OcrSettings>) {
    const next = { ...s, ...patch };
    setS(next);
    onChange(next);
  }

  return (
    <div className="space-y-3 p-4 border rounded-lg bg-gray-50">
      <h3 className="font-semibold text-sm">OCR Settings</h3>
      <div className="grid grid-cols-2 gap-3 text-sm">
        <label className="col-span-2 flex flex-col gap-1">
          Engine
          <select value={s.engine} onChange={e => update({ engine: e.target.value })}
            className="border rounded px-2 py-1">
            <option value="rapidocr">RapidOCR</option>
            <option value="easyocr">EasyOCR</option>
            <option value="tesseract">Tesseract</option>
            <option value="mac">Mac OCR</option>
          </select>
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={s.do_ocr} onChange={e => update({ do_ocr: e.target.checked })} />
          Enable OCR
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={s.do_table_structure} onChange={e => update({ do_table_structure: e.target.checked })} />
          Table structure
        </label>
        <label className="flex flex-col gap-1">
          Table mode
          <select value={s.table_mode} onChange={e => update({ table_mode: e.target.value })}
            className="border rounded px-2 py-1">
            <option value="fast">Fast</option>
            <option value="accurate">Accurate</option>
          </select>
        </label>
        <label className="flex flex-col gap-1">
          Image scale
          <input type="number" value={s.images_scale} step={0.5} min={1} max={4}
            onChange={e => update({ images_scale: parseFloat(e.target.value) })}
            className="border rounded px-2 py-1" />
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={s.generate_picture_images} onChange={e => update({ generate_picture_images: e.target.checked })} />
          Extract images
        </label>
        <label className="flex flex-col gap-1">
          Timeout (s)
          <input type="number" value={s.document_timeout} min={30} max={1800}
            onChange={e => update({ document_timeout: parseInt(e.target.value) })}
            className="border rounded px-2 py-1" />
        </label>
      </div>
    </div>
  );
}
