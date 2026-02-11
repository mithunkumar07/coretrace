# CoreTrace Agent Build and Deployment

## Makefile Commands

### Building
```bash
# Standard build
make build

# Debug build with symbols
make build-debug

# Optimized release build
make build-release

# Multi-architecture build
make build-all
```

### Docker
```bash
# Build Docker image
make docker-build

# Build optimized Docker image
make docker-build-release

# Push to registry
make docker-push VERSION=v1.0.0

# Run in Docker
make docker-run
```

### Development
```bash
# Install dependencies
make deps

# Run tests
make test

# Test with coverage
make test-coverage

# Lint code
make lint

# Format code
make format

# Run in development mode
make dev-run

# Install system-wide
make install
```

## Docker Usage

### Development
```bash
# Build and run development container
docker-compose up --build

# Run with SSH test client
docker-compose --profile testing up
```

### Production
```bash
# Build production image
docker build -t coretrace/agent:v1.0.0 .

# Run with necessary privileges
docker run -d \
  --name coretrace-agent \
  --privileged \
  -v /var/log:/var/log:ro \
  -v /etc/coretrace:/etc/coretrace:ro \
  coretrace/agent:v1.0.0 monitor

# Or run with custom config
docker run -d \
  --name coretrace-agent \
  --privileged \
  -v /var/log:/var/log:ro \
  -v ./config.yaml:/etc/coretrace/config.yaml:ro \
  coretrace/agent:v1.0.0 monitor --debug
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: coretrace-agent
spec:
  selector:
    matchLabels:
      app: coretrace-agent
  template:
    metadata:
      labels:
        app: coretrace-agent
    spec:
      hostPID: true
      hostNetwork: true
      containers:
      - name: agent
        image: coretrace/agent:v1.0.0
        securityContext:
          privileged: true
        volumeMounts:
        - name: log-volume
          mountPath: /var/log
          readOnly: true
        - name: config-volume
          mountPath: /etc/coretrace
          readOnly: true
      volumes:
      - name: log-volume
        hostPath:
          path: /var/log
      - name: config-volume
        configMap:
          name: coretrace-config
```

## Configuration

### Environment Variables
- `VERSION`: Build version
- `BUILD_TIME`: Build timestamp
- `COMMIT_SHA`: Git commit SHA
- `DOCKER_REGISTRY`: Docker registry (default: coretrace)
- `DOCKER_TAG`: Docker tag (default: git tag or "dev")

### Mount Points
- `/var/log`: Host logs (read-only)
- `/etc/coretrace`: Configuration directory
- `/proc`: Process information (read-only)
- `/host/proc`: Alternative proc mount

### Security Notes
- Requires privileged mode for file system monitoring
- Needs read access to `/var/log/auth.log` or `/var/log/secure`
- Consider using service accounts in production
- Enable only necessary capabilities in production

## Release Process

```bash
# Create release
make tag VERSION=v1.0.0

# Build release artifacts
make build-release
make docker-build-release

# Publish
make docker-push DOCKER_TAG=v1.0.0
```