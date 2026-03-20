import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import type { Event, WebSocketMessage } from '../types';

const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080';

interface WebSocketContextType {
  connected: boolean;
  recentEvents: Event[];
}

const WebSocketContext = createContext<WebSocketContextType>({ connected: false, recentEvents: [] });

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const [connected, setConnected] = useState(false);
  const [recentEvents, setRecentEvents] = useState<Event[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelay = useRef(1000);
  const mountedRef = useRef(true);

  const connect = () => {
    if (!mountedRef.current) return;
    try {
      const ws = new WebSocket(`${WS_URL}/ws/dashboard`);
      wsRef.current = ws;

      ws.onopen = () => {
        if (!mountedRef.current) return;
        setConnected(true);
        reconnectDelay.current = 1000;
      };

      ws.onclose = () => {
        if (!mountedRef.current) return;
        setConnected(false);
        reconnectTimer.current = setTimeout(() => {
          reconnectDelay.current = Math.min(reconnectDelay.current * 2, 60000);
          connect();
        }, reconnectDelay.current);
      };

      ws.onerror = () => {
        ws.close();
      };

      ws.onmessage = (e) => {
        try {
          const msg: WebSocketMessage = JSON.parse(e.data);
          if (msg.type === 'event' || msg.event_type) {
            const event: Event = {
              id: crypto.randomUUID(),
              agent_id: msg.agent_id || '',
              event_type: msg.event_type || msg.type,
              timestamp: msg.timestamp || new Date().toISOString(),
              severity: (msg.severity as Event['severity']) || 'info',
              data: msg.data || {},
              session_id: msg.session_id,
            };
            setRecentEvents(prev => [event, ...prev].slice(0, 100));
          }
        } catch {
          // ignore malformed messages
        }
      };
    } catch {
      reconnectTimer.current = setTimeout(connect, reconnectDelay.current);
    }
  };

  useEffect(() => {
    mountedRef.current = true;
    connect();
    return () => {
      mountedRef.current = false;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
    };
  }, []);

  return (
    <WebSocketContext.Provider value={{ connected, recentEvents }}>
      {children}
    </WebSocketContext.Provider>
  );
}

export function useWebSocket() {
  return useContext(WebSocketContext);
}
