import { useEffect, useState } from 'react';
import { useWebSocket } from '../context/WebSocketContext';
import { useAuth } from '../context/AuthContext';
import type { DashboardStats, Event } from '../types';
import { API_BASE, SEVERITY_CLASS, apiFetch } from '../utils/api';

export default function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const { connected, recentEvents } = useWebSocket();
  const { token } = useAuth();

  useEffect(() => {
    const load = () => {
      apiFetch(`${API_BASE}/api/v1/stats`, token)
        .then(r => r.json())
        .then(setStats)
        .catch(() => {});
    };
    load();
    const interval = setInterval(load, 15000);
    return () => clearInterval(interval);
  }, [token]);

  return (
    <div className="view">
      <div className="view-header">
        <h2>Dashboard</h2>
        <span className={`ws-badge ${connected ? 'ws-connected' : 'ws-disconnected'}`}>
          {connected ? '● Live' : '○ Disconnected'}
        </span>
      </div>

      <div className="stats-grid">
        <StatCard label="Total Agents" value={stats?.total_agents ?? '–'} />
        <StatCard label="Online Agents" value={stats?.online_agents ?? '–'} variant="success" />
        <StatCard label="Events (24h)" value={stats?.total_events_24h ?? '–'} variant="warning" />
        <StatCard label="Active Sessions" value={stats?.active_sessions ?? '–'} variant="info" />
      </div>

      <section className="section">
        <h3>Recent Events</h3>
        {recentEvents.length === 0 ? (
          <p className="empty-state">No live events yet. Events from connected agents will appear here in real-time.</p>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Type</th>
                <th>Severity</th>
                <th>Agent</th>
              </tr>
            </thead>
            <tbody>
              {recentEvents.slice(0, 20).map((ev: Event) => (
                <tr key={ev.id}>
                  <td className="mono">{new Date(ev.timestamp).toLocaleTimeString()}</td>
                  <td className="mono">{ev.event_type}</td>
                  <td>
                    <span className={`badge ${SEVERITY_CLASS[ev.severity] || 'badge-info'}`}>
                      {ev.severity}
                    </span>
                  </td>
                  <td className="mono text-secondary">
                    {ev.agent_id ? ev.agent_id.slice(0, 8) + '…' : '–'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}

function StatCard({ label, value, variant }: { label: string; value: number | string; variant?: string }) {
  return (
    <div className={`stat-card ${variant ? `stat-card--${variant}` : ''}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}
