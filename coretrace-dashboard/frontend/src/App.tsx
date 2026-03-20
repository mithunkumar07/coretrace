import { useState } from 'react';
import './App.css';
import Dashboard from './components/Dashboard';
import Agents from './components/Agents';
import Events from './components/Events';
import Sessions from './components/Sessions';
import Login from './components/Login';
import Sidebar from './components/Sidebar';
import { WebSocketProvider } from './context/WebSocketContext';
import { AuthProvider, useAuth } from './context/AuthContext';

function AppContent() {
  const [currentView, setCurrentView] = useState('dashboard');
  const { isAuthenticated, logout } = useAuth();

  if (!isAuthenticated) {
    return <Login />;
  }

  return (
    <WebSocketProvider>
      <div className="app">
        <Sidebar currentView={currentView} onViewChange={setCurrentView} onLogout={logout} />
        <main className="main-content">
          {currentView === 'dashboard' && <Dashboard />}
          {currentView === 'agents' && <Agents />}
          {currentView === 'events' && <Events />}
          {currentView === 'sessions' && <Sessions />}
        </main>
      </div>
    </WebSocketProvider>
  );
}

function App() {
  return (
    <AuthProvider>
      <AppContent />
    </AuthProvider>
  );
}

export default App;
