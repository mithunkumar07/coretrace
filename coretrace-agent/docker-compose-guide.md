# Docker Compose Usage Guide

## Environment Setup

Copy the example environment file:
```bash
cp .env.example .env
```

## Available Profiles

### 1. Production (Default)
```bash
# Start the production agent
docker-compose up -d

# View logs
docker-compose logs -f coretrace-agent

# Stop the agent
docker-compose down
```

### 2. Development Profile
```bash
# Start development environment with hot reload
docker-compose --profile dev up -d

# Run with local source code mounting
docker-compose --profile dev up --build
```

### 3. Testing Profile
```bash
# Start testing environment with SSH test server
docker-compose --profile testing up -d

# This includes:
# - coretrace-agent-test (testing configuration)
# - ssh-test-server (SSH server on port 2222)
# - ssh-test-client (for making test connections)
# - log-simulator (for testing log parsing)

# Run tests
docker-compose --profile testing exec coretrace-agent-test /usr/local/bin/coretrace-agent --debug

# Test SSH connection
docker-compose --profile testing exec ssh-test-client ssh testuser@ssh-test-server -p 2222
```

### 4. Full Stack Profile
```bash
# Start full application stack with collector and database
docker-compose --profile full up -d

# This includes all services:
# - coretrace-agent (production)
# - collector-agent (data collection service)
# - redis (caching/queue)
# - postgres (data storage)
```

## Service Details

### CoreTrace Agent
- **Container Name**: `coretrace-agent`
- **Ports**: None (agent only, no network exposure needed)
- **Volumes**: 
  - `/var/log` (host logs for SSH monitoring)
  - `/etc/coretrace/config.yaml` (configuration)
  - Various system directories for file monitoring
- **Privileged**: Required for system access

### SSH Test Server (Testing)
- **Container Name**: `ssh-test-server`
- **Ports**: `2222:2222`
- **Credentials**: `testuser:test123`
- **Purpose**: Testing SSH connection monitoring

### Redis (Full Stack)
- **Container Name**: `coretrace-redis`
- **Ports**: `6379:6379`
- **Purpose**: Caching and message queue

### PostgreSQL (Full Stack)
- **Container Name**: `coretrace-postgres`
- **Ports**: `5432:5432`
- **Database**: `coretrace`
- **Credentials**: `coretrace:password`

## Development Workflow

1. **Start Development Environment**:
   ```bash
   docker-compose --profile dev up -d
   ```

2. **Make Code Changes**:
   - Changes are hot-reloaded from local directory
   - Configuration changes are persisted

3. **Test Changes**:
   ```bash
   docker-compose --profile testing up -d
   # Run integration tests
   ```

4. **Build for Production**:
   ```bash
   export VERSION=v1.0.0
   export DOCKER_TAG=v1.0.0
   docker-compose build --no-cache
   ```

## Monitoring

### Check Agent Status
```bash
docker-compose ps
docker-compose logs -f coretrace-agent
```

### Health Checks
```bash
docker inspect coretrace-agent | grep Health -A 10
```

### Debug Container
```bash
docker-compose exec coretrace-agent /bin/sh
```

## Production Deployment

1. **Set Environment Variables**:
   ```bash
   export ENVIRONMENT=production
   export VERSION=v1.0.0
   export DOCKER_TAG=v1.0.0
   export LOG_LEVEL=info
   export DEBUG=false
   ```

2. **Deploy**:
   ```bash
   docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
   ```

3. **Monitor**:
   ```bash
   docker-compose logs -f coretrace-agent
   ```

## Troubleshooting

### Permission Issues
```bash
# Ensure proper permissions for log directories
sudo chmod 755 /var/log
sudo chown root:adm /var/log/auth.log
```

### Volume Mount Issues
```bash
# Check if host directories exist
ls -la /var/log/auth.log
ls -la /var/log/secure
```

### Network Issues
```bash
# Test network connectivity
docker network ls
docker network inspect coretrace-agent_coretrace-net
```

### Container Issues
```bash
# View detailed container info
docker inspect coretrace-agent

# Run container in interactive mode for debugging
docker run -it --rm --privileged -v /var/log:/var/log:ro coretrace/agent:dev /bin/sh
```