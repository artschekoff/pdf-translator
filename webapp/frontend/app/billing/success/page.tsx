"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";

export default function BillingSuccessPage() {
  const router = useRouter();
  const [balance, setBalance] = useState<number | null>(null);
  const [credited, setCredited] = useState<number | null>(null);
  const [syncing, setSyncing] = useState(true);

  useEffect(() => {
    let attempts = 0;
    const maxAttempts = 6;
    const delayMs = 2000;

    async function sync() {
      try {
        const result = await api.syncPurchases();
        setBalance(result.balance);
        if (result.credited > 0) {
          setCredited(result.credited);
          setSyncing(false);
          return;
        }
      } catch {
        // ignore, retry
      }

      attempts++;
      if (attempts < maxAttempts) {
        setTimeout(sync, delayMs);
      } else {
        // Give up polling — show balance as-is
        api.getBalance().then(r => setBalance(r.pages)).catch(() => {});
        setSyncing(false);
      }
    }

    sync();
  }, []);

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="bg-white rounded-2xl shadow p-10 text-center max-w-md">
        {syncing && credited === null ? (
          <>
            <div className="text-4xl mb-4 animate-pulse">⏳</div>
            <h1 className="text-xl font-semibold mb-2">Confirming payment…</h1>
            <p className="text-gray-400 text-sm">This takes just a moment.</p>
          </>
        ) : (
          <>
            <div className="text-5xl mb-4">✓</div>
            <h1 className="text-2xl font-bold mb-2">Payment successful</h1>
            <p className="text-gray-500 mb-6">
              {credited !== null && credited > 0
                ? <><strong>{credited} pages</strong> have been added to your account.</>
                : <>Your account has been updated.</>}
              {balance !== null && (
                <> You now have <strong>{balance} pages</strong> available.</>
              )}
            </p>
            <div className="flex gap-3 justify-center">
              <button
                onClick={() => router.push("/documents/upload")}
                className="bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700"
              >
                Translate a PDF
              </button>
              <button
                onClick={() => router.push("/billing")}
                className="border px-6 py-2 rounded-lg hover:bg-gray-50"
              >
                Buy more
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
