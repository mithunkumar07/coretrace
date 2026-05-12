import { useEffect, useState, useCallback } from 'react';
import { useAuth } from '../context/AuthContext';
import type { Agent } from '../types';
import { API_BASE, apiFetch } from '../utils/api';

const STATUS_CLASS: Record<string, string> = {
  online: 'badge-success',
  offline: 'badge-secondary',
  error: 'badge-error',
};

function formatLastSeen(ts: string) {
  if (!ts) return '–';
  const diff = Date.now() - new Date(ts).getTime();
  if (diff < 60000) return 'just now';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
  return new Date(ts).toLocaleDateString();
}

export default function Agents() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const { token } = useAuth();

  const load = useCallback(() => {
    apiFetch(`${API_BASE}/api/v1/agents`, token)
      .then(r => r.json())
      .then(data => { setAgents(Array.isArray(data) ? data : []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [token]);

  useEffect(() => {
    load();
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [load]);

  return (
    <div className="view">
      <div className="view-header">
        <h2>Agents</h2>
        <button className="btn-secondary" onClick={load}>Refresh</button>
      </div>

      {loading ? (
        <p className="empty-state">Loading agents…</p>
      ) : agents.length === 0 ? (
        <p className="empty-state">
          No agents registered. Deploy the CoreTrace agent and configure it to connect to this dashboard.
        </p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Hostname</th>
              <th>IP Address</th>
              <th>Version</th>
              <th>Status</th>
              <th>Last Seen</th>
            </tr>
          </thead>
          <tbody>
            {agents.map(agent => (
              <tr key={agent.id}>
                <td className="font-medium">{agent.name || agent.hostname}</td>
                <td className="mono">{agent.hostname}</td>
                <td className="mono">{agent.ip_address}</td>
                <td className="mono text-secondary">{agent.version || '–'}</td>
                <td>
                  <span className={`badge ${STATUS_CLASS[agent.status] || 'badge-secondary'}`}>
                    {agent.status}
                  </span>
                </td>
                <td className="text-secondary">{formatLastSeen(agent.last_seen)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
