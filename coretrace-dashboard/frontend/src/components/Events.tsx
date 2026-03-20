import { useEffect, useState } from 'react';
import type { Event } from '../types';
import { useWebSocket } from '../context/WebSocketContext';

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080';

const SEVERITY_CLASS: Record<string, string> = {
  info: 'badge-info',
  warning: 'badge-warning',
  error: 'badge-error',
  critical: 'badge-critical',
};

export default function Events() {
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterType, setFilterType] = useState('');
  const [filterSeverity, setFilterSeverity] = useState('');
  const { recentEvents } = useWebSocket();

  const load = () => {
    const params = new URLSearchParams();
    if (filterType) params.set('type', filterType);
    if (filterSeverity) params.set('severity', filterSeverity);
    fetch(`${API_BASE}/api/v1/events?${params}`)
      .then(r => r.json())
      .then(data => { setEvents(data.events || []); setLoading(false); })
      .catch(() => setLoading(false));
  };

  useEffect(() => { load(); }, [filterType, filterSeverity]);

  // Merge live WS events in real-time
  useEffect(() => {
    if (recentEvents.length === 0) return;
    setEvents(prev => {
      const ids = new Set(prev.map(e => e.id));
      const newOnes = recentEvents.filter(e => !ids.has(e.id));
      if (newOnes.length === 0) return prev;
      return [...newOnes, ...prev].slice(0, 200);
    });
  }, [recentEvents]);

  return (
    <div className="view">
      <div className="view-header">
        <h2>Events</h2>
        <div className="filters">
          <select value={filterType} onChange={e => setFilterType(e.target.value)}>
            <option value="">All Types</option>
            <option value="ssh_login">SSH Login</option>
            <option value="ssh_logout">SSH Logout</option>
            <option value="ssh_failed">SSH Failed</option>
            <option value="command">Command</option>
            <option value="file_create">File Create</option>
            <option value="file_modify">File Modify</option>
            <option value="file_delete">File Delete</option>
          </select>
          <select value={filterSeverity} onChange={e => setFilterSeverity(e.target.value)}>
            <option value="">All Severities</option>
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
            <option value="critical">Critical</option>
          </select>
        </div>
      </div>

      {loading ? (
        <p className="empty-state">Loading events…</p>
      ) : events.length === 0 ? (
        <p className="empty-state">No events found.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Time</th>
              <th>Type</th>
              <th>Severity</th>
              <th>Agent</th>
              <th>Session</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            {events.map(ev => (
              <tr key={ev.id}>
                <td className="mono">{new Date(ev.timestamp).toLocaleString()}</td>
                <td className="mono">{ev.event_type}</td>
                <td>
                  <span className={`badge ${SEVERITY_CLASS[ev.severity] || 'badge-info'}`}>
                    {ev.severity}
                  </span>
                </td>
                <td className="mono text-secondary">
                  {ev.agent_id ? ev.agent_id.slice(0, 8) + '…' : '–'}
                </td>
                <td className="mono text-secondary">
                  {ev.session_id ? ev.session_id.slice(0, 8) + '…' : '–'}
                </td>
                <td className="text-secondary truncate">
                  {JSON.stringify(ev.data).slice(0, 60)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
