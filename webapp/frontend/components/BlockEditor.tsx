"use client";
import { useEffect, useRef, useCallback, useImperativeHandle, forwardRef } from "react";
import type { BlockData } from "@/lib/api";

const SAVE_DEBOUNCE_MS = 1000;

export interface BlockEditorHandle {
  save: () => Promise<BlockData[]>;
  load: (blocks: BlockData[]) => Promise<void>;
}

interface Props {
  initialBlocks?: BlockData[];
  onSave: (blocks: BlockData[]) => void;
  onBlockSelect?: (text: string) => void;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type EditorInstance = any;

function toEditorBlocks(blocks: BlockData[]) {
  return blocks.map(b => ({
    id: b.id,
    type: b.type,
    data: b.data,
  }));
}

function fromEditorBlocks(blocks: { id?: string; type: string; data: Record<string, unknown> }[]): BlockData[] {
  return blocks.map((b, i) => ({
    id: b.id ?? `block_${i}`,
    type: b.type,
    data: b.data,
    order: i,
  }));
}

const BlockEditorInner = forwardRef<BlockEditorHandle, Props>(
  function BlockEditorInner({ initialBlocks, onSave, onBlockSelect }, ref) {
    const holderRef = useRef<HTMLDivElement>(null);
    const editorRef = useRef<EditorInstance>(null);
    const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const initializedRef = useRef(false);

    const scheduleSave = useCallback(async () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      saveTimerRef.current = setTimeout(async () => {
        if (!editorRef.current) return;
        try {
          const out = await editorRef.current.save();
          onSave(fromEditorBlocks(out.blocks));
        } catch {}
      }, SAVE_DEBOUNCE_MS);
    }, [onSave]);

    useImperativeHandle(ref, () => ({
      save: async () => {
        if (!editorRef.current) return [];
        try {
          const out = await editorRef.current.save();
          return fromEditorBlocks(out.blocks);
        } catch { return []; }
      },
      load: async (blocks: BlockData[]) => {
        if (!editorRef.current) return;
        try {
          await editorRef.current.render({ blocks: toEditorBlocks(blocks) });
        } catch {}
      },
    }));

    useEffect(() => {
      if (typeof window === "undefined") return;
      if (initializedRef.current) return;
      initializedRef.current = true;

      let destroyed = false;

      (async () => {
        const [
          { default: EditorJS },
          { default: Header },
          { default: List },
          { default: Code },
          { default: Quote },
          { default: Delimiter },
          { default: Table },
          { default: Checklist },
        ] = await Promise.all([
          import("@editorjs/editorjs"),
          import("@editorjs/header"),
          import("@editorjs/list"),
          import("@editorjs/code"),
          import("@editorjs/quote"),
          import("@editorjs/delimiter"),
          import("@editorjs/table"),
          import("@editorjs/checklist"),
        ]);

        if (destroyed || !holderRef.current) return;

        editorRef.current = new EditorJS({
          holder: holderRef.current,
          autofocus: false,
          placeholder: "Start writing…",
          data: initialBlocks && initialBlocks.length > 0
            ? { blocks: toEditorBlocks(initialBlocks) }
            : undefined,
          tools: {
            header: {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              class: Header as any,
              config: { levels: [1, 2, 3], defaultLevel: 2 },
            },
            list: {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              class: List as any,
              inlineToolbar: true,
              config: { defaultStyle: "unordered" },
            },
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            code: { class: Code as any },
            quote: {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              class: Quote as any,
              inlineToolbar: true,
            },
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            delimiter: { class: Delimiter as any },
            table: {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              class: Table as any,
              inlineToolbar: true,
            },
            checklist: {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              class: Checklist as any,
              inlineToolbar: true,
            },
          },
          onChange: () => {
            scheduleSave();

            // Capture selected block text for AI panel
            if (onBlockSelect) {
              try {
                const idx = editorRef.current?.blocks?.getCurrentBlockIndex?.() ?? -1;
                if (idx >= 0) {
                  editorRef.current?.save?.().then((out: { blocks: Array<{ data: { text?: string; items?: string[] } }> }) => {
                    const block = out.blocks[idx];
                    if (!block) return;
                    let text = "";
                    if (typeof block.data.text === "string") {
                      text = block.data.text;
                    } else if (Array.isArray(block.data.items)) {
                      text = (block.data.items as string[]).join("\n");
                    }
                    onBlockSelect(text.replace(/<[^>]*>/g, "").trim());
                  }).catch(() => {});
                }
              } catch {}
            }
          },
        });
      })();

      return () => {
        destroyed = true;
        if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
        if (editorRef.current?.destroy) {
          try { editorRef.current.destroy(); } catch {}
          editorRef.current = null;
        }
        initializedRef.current = false;
      };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    return (
      <div
        ref={holderRef}
        className="min-h-[400px] prose prose-sm max-w-none focus:outline-none"
      />
    );
  }
);

BlockEditorInner.displayName = "BlockEditorInner";

// Public wrapper — guards SSR
export function BlockEditor(props: Props & { ref?: React.Ref<BlockEditorHandle> }) {
  if (typeof window === "undefined") {
    return <div className="min-h-[400px] animate-pulse bg-gray-100 rounded" />;
  }
  const { ref: _ref, ...rest } = props;
  return <BlockEditorInner ref={_ref} {...rest} />;
}
