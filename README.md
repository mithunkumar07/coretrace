# CoreTrace Monorepo

This is a monorepo containing all CoreTrace applications for Infrastructure Runtime Intelligence.

## Applications

### `coretrace-agent/`
SSH session monitoring agent for Linux systems. Monitors:
- SSH login attempts (successful/failed)
- Command execution within sessions
- File access and modifications
- Session correlation and attribution

### `coretrace-collector/` (Coming Soon)
Customer-hosted data plane component for local event collection, processing, and storage.

### `coretrace-control-plane/` (Coming Soon)
SaaS control plane for multi-tenant management, policy enforcement, and intelligence layer.

## Repository Structure

```
├── coretrace-agent/          # SSH monitoring agent
├── coretrace-collector/      # Data plane collector
├── coretrace-control-plane/  # SaaS control plane
├── shared/                   # Shared libraries and types
├── docs/                     # Documentation
└── deploy/                   # Deployment configurations
```

## Quick Start

Each application has its own README and build instructions in its respective directory.

## Development

This monorepo uses Go modules with relative imports. Each application is a standalone Go module within the repository.