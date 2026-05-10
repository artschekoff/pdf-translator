const AUTH_URL = process.env.NEXT_PUBLIC_AUTH_URL ?? "http://localhost:8002";

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export interface LoginResponse {
  access_token: string | null;
  refresh_token: string | null;
  token_type: string;
  mfa_required: boolean;
  mfa_token: string | null;
}

export async function login(email: string, password: string): Promise<LoginResponse> {
  const res = await fetch(`${AUTH_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function refreshTokens(refreshToken: string): Promise<AuthTokens> {
  const res = await fetch(`${AUTH_URL}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  if (!res.ok) throw new Error("Session expired");
  return res.json();
}

export const tokenStore = {
  get: () => (typeof window !== "undefined" ? localStorage.getItem("access_token") ?? "" : ""),
  set: (t: AuthTokens) => {
    localStorage.setItem("access_token", t.access_token);
    localStorage.setItem("refresh_token", t.refresh_token);
  },
  clear: () => {
    localStorage.removeItem("access_token");
    localStorage.removeItem("refresh_token");
  },
};
