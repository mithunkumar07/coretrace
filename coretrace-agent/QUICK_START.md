# Quick Start Guide

## Stack Management Script

The `stack.sh` script provides easy management of the CoreTrace Docker stack.

### Available Commands

```bash
# Start environments
./stack.sh start prod          # Production (default)
./stack.sh start dev           # Development with hot reload
./stack.sh start test          # Testing with SSH server
./stack.sh start full          # Full stack with database

# Stop environments
./stack.sh stop prod
./stack.sh stop dev
./stack.sh stop test
./stack.sh stop full

# Restart environments
./stack.sh restart prod
./stack.sh restart dev
./stack.sh restart test

# Check status
./stack.sh status prod
./stack.sh status test

# View logs
./stack.sh logs prod           # All logs
./stack.sh logs prod agent      # Specific service logs
./stack.sh logs test           # Testing environment logs

# Get help
./stack.sh --help
```

## Environment Profiles

### Production (`prod`)
- Optimized configuration
- Health checks enabled
- Resource limits applied
- Logging with rotation
- Production environment variables

### Development (`dev`)
- Debug logging enabled
- Hot reload support
- Source code mounting
- Development tools
- No restart policy

### Testing (`test`)
- Test configuration
- SSH test server on port 2222
- SSH client container
- Debug logging
- Test environment variables

### Full Stack (`full`)
- All services including:
  - CoreTrace Agent
  - Collector Service
  - Redis (caching)
  - PostgreSQL (storage)

## Environment Variables

Copy `.env.example` to `.env` and customize:

```bash
# Docker settings
DOCKER_REGISTRY=coretrace
VERSION=v1.0.0
DOCKER_TAG=v1.0.0

# Agent settings
AGENT_NAME=coretrace-agent
LOG_LEVEL=info
DEBUG=false
```

## Quick Testing

1. **Start Testing Environment:**
   ```bash
   ./stack.sh start test
   ```

2. **Test SSH Connection:**
   ```bash
   ssh testuser@localhost -p 2222
   # Password: test123
   ```

3. **Monitor Logs:**
   ```bash
   ./stack.sh logs test coretrace-agent
   ```

4. **Stop Testing:**
   ```bash
   ./stack.sh stop test
   ```

## Production Deployment

1. **Set Production Variables:**
   ```bash
   export VERSION=v1.0.0
   export LOG_LEVEL=info
   export DEBUG=false
   ```

2. **Deploy:**
   ```bash
   ./stack.sh start prod
   ```

3. **Monitor:**
   ```bash
   ./stack.sh status prod
   ./stack.sh logs prod
   ```

## Troubleshooting

### Permission Issues
```bash
# Ensure proper log permissions
sudo chmod 755 /var/log
sudo chown root:adm /var/log/auth.log
```

### Docker Issues
```bash
# Check Docker status
docker info

# Reset Docker stack
./stack.sh stop prod
docker system prune -f
./stack.sh start prod
```

### Network Issues
```bash
# Check networks
docker network ls
docker network inspect coretrace-agent_coretrace-net

# Reset network
docker network rm coretrace-agent_coretrace-net
./stack.sh start prod
```

## Health Monitoring

The stack includes built-in health checks:

- **Agent Health**: `pgrep coretrace-agent`
- **Check Interval**: 30 seconds
- **Timeout**: 10 seconds
- **Retries**: 3

View health status:
```bash
docker inspect coretrace-agent-prod | grep Health -A 10
```