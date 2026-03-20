export interface Agent {
  id: string;
  name: string;
  hostname: string;
  ip_address: string;
  version: string;
  status: 'online' | 'offline' | 'error';
  last_seen: string;
  created_at: string;
  updated_at: string;
  metadata?: Record<string, any>;
}

export interface Event {
  id: string;
  agent_id: string;
  agent?: Agent;
  event_type: string;
  timestamp: string;
  severity: 'info' | 'warning' | 'error' | 'critical';
  data: Record<string, any>;
  session_id?: string;
}

export interface Session {
  id: string;
  agent_id: string;
  agent?: Agent;
  session_id: string;
  username: string;
  source_ip: string;
  auth_method: string;
  login_time: string;
  logout_time?: string;
  command_count: number;
  status: 'active' | 'closed' | 'timeout';
  key_fingerprint?: string;
}

export interface DashboardStats {
  total_agents: number;
  online_agents: number;
  total_events_24h: number;
  active_sessions: number;
  last_updated: string;
}

export interface User {
  id: string;
  email: string;
  name: string;
  role: 'admin' | 'viewer' | 'operator';
}

export interface WebSocketMessage {
  type: string;
  timestamp: string;
  event_type?: string;
  data?: any;
  agent_id?: string;
  session_id?: string;
  severity?: string;
}
