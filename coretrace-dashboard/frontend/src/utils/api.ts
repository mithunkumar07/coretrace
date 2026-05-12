export const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080';

export const SEVERITY_CLASS: Record<string, string> = {
  info: 'badge-info',
  warning: 'badge-warning',
  error: 'badge-error',
  critical: 'badge-critical',
};

export function apiFetch(url: string, token: string | null, init?: RequestInit): Promise<Response> {
  return fetch(url, {
    ...init,
    headers: {
      ...(init?.headers as Record<string, string>),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
}
