interface SidebarProps {
  currentView: string;
  onViewChange: (view: string) => void;
  onLogout: () => void;
}

const navItems = [
  { id: 'dashboard', label: 'Dashboard', icon: '▦' },
  { id: 'agents', label: 'Agents', icon: '⬡' },
  { id: 'events', label: 'Events', icon: '⚡' },
  { id: 'sessions', label: 'Sessions', icon: '⊡' },
];

export default function Sidebar({ currentView, onViewChange, onLogout }: SidebarProps) {
  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <div className="brand-logo">CT</div>
        <span className="brand-name">CoreTrace</span>
      </div>
      <nav className="sidebar-nav">
        {navItems.map(item => (
          <button
            key={item.id}
            className={`nav-item ${currentView === item.id ? 'active' : ''}`}
            onClick={() => onViewChange(item.id)}
          >
            <span className="nav-icon">{item.icon}</span>
            <span>{item.label}</span>
          </button>
        ))}
      </nav>
      <div className="sidebar-footer">
        <button className="nav-item" onClick={onLogout}>
          <span className="nav-icon">↩</span>
          <span>Logout</span>
        </button>
      </div>
    </aside>
  );
}
