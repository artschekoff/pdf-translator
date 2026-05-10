"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, BillingPlan } from "@/lib/api";
import { tokenStore } from "@/lib/auth";

export default function BillingPage() {
  const router = useRouter();
  const [plans, setPlans] = useState<BillingPlan[]>([]);
  const [balance, setBalance] = useState<number | null>(null);
  const [loading, setLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!tokenStore.get()) { router.replace("/login"); return; }
    api.listPlans().then(setPlans).catch(() => setError("Failed to load plans"));
    api.getBalance().then(r => setBalance(r.pages)).catch(() => {});
  }, [router]);

  async function handleBuy(planId: string) {
    setLoading(planId);
    setError(null);
    try {
      const { url } = await api.createCheckout(planId);
      window.location.href = url;
    } catch {
      setError("Failed to start checkout. Please try again.");
      setLoading(null);
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      <div className="max-w-4xl mx-auto">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold">Buy Pages</h1>
            <p className="text-gray-500 mt-1">Credits are added instantly after payment.</p>
          </div>
          {balance !== null && (
            <div className="bg-white border rounded-xl px-6 py-3 text-right">
              <div className="text-2xl font-bold text-blue-600">{balance}</div>
              <div className="text-sm text-gray-500">pages remaining</div>
            </div>
          )}
        </div>

        {error && <div className="mb-6 text-red-600 bg-red-50 border border-red-200 rounded-lg p-4">{error}</div>}

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {plans.map(plan => (
            <div key={plan.id} className="bg-white rounded-2xl shadow p-6 flex flex-col">
              <div className="text-lg font-semibold mb-1">{plan.name}</div>
              <div className="text-3xl font-bold mb-1">
                ${(plan.price_cents / 100).toFixed(0)}
              </div>
              <div className="text-gray-400 text-sm mb-6">
                {plan.pages.toLocaleString()} pages
                <span className="ml-2 text-gray-300">
                  · ${(plan.price_cents / plan.pages / 100).toFixed(2)}/page
                </span>
              </div>
              <button
                onClick={() => handleBuy(plan.id)}
                disabled={loading === plan.id}
                className="mt-auto w-full bg-blue-600 text-white py-2 rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {loading === plan.id ? "Redirecting…" : "Buy now"}
              </button>
            </div>
          ))}
        </div>

        <p className="mt-8 text-xs text-gray-400 text-center">
          Payments are processed securely by Stripe. Pages are non-refundable once used.
        </p>
      </div>
    </div>
  );
}
