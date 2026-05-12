import { useEffect, useState, useCallback } from 'react';
import { useAuth } from '../context/AuthContext';
import type { Session } from '../types';
import { API_BASE, apiFetch } from '../utils/api';

const STATUS_CLASS: Record<string, string> = {
  active: 'badge-success',
  closed: 'badge-secondary',
  timeout: 'badge-warning',
};

function formatDuration(start: string, end?: string) {
  const from = new Date(start).getTime();
  const to = end ? new Date(end).getTime() : Date.now();
  const diff = Math.floor((to - from) / 1000);
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`;
  return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`;
}

export default function Sessions() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<'all' | 'active'>('all');
  const { token } = useAuth();

  const load = useCallback(() => {
    const url = view === 'active'
      ? `${API_BASE}/api/v1/sessions/active`
      : `${API_BASE}/api/v1/sessions`;
    apiFetch(url, token)
      .then(r => r.json())
      .then(data => { setSessions(Array.isArray(data) ? data : []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [view, token]);

  useEffect(() => {
    load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, [load]);

  return (
    <div className="view">
      <div className="view-header">
        <h2>Sessions</h2>
        <div className="filters">
          <button
            className={`btn-tab ${view === 'all' ? 'active' : ''}`}
            onClick={() => setView('all')}
          >
            All
          </button>
          <button
            className={`btn-tab ${view === 'active' ? 'active' : ''}`}
            onClick={() => setView('active')}
          >
            Active
          </button>
        </div>
      </div>

      {loading ? (
        <p className="empty-state">Loading sessions…</p>
      ) : sessions.length === 0 ? (
        <p className="empty-state">No sessions found.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>User</th>
              <th>Source IP</th>
              <th>Auth Method</th>
              <th>Login Time</th>
              <th>Duration</th>
              <th>Commands</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {sessions.map(s => (
              <tr key={s.id}>
                <td className="font-medium">{s.username}</td>
                <td className="mono">{s.source_ip}</td>
                <td className="mono text-secondary">{s.auth_method}</td>
                <td className="text-secondary">{new Date(s.login_time).toLocaleString()}</td>
                <td className="mono">{formatDuration(s.login_time, s.logout_time)}</td>
                <td>{s.command_count}</td>
                <td>
                  <span className={`badge ${STATUS_CLASS[s.status] || 'badge-secondary'}`}>
                    {s.status}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
