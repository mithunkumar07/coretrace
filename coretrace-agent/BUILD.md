# CoreTrace Agent Build Instructions

## Quick Build

```bash
cd coretrace-agent
go build -o coretrace-agent .
```

That's it! The agent is a single static binary with no external Go dependencies at runtime.

## Requirements

### Build-time
- Go 1.21 or later
- Linux (for system call constants)

### Runtime
- Linux (any modern distribution)
- Root privileges (for reading system logs and /proc)
- Kernel 2.6.32+ (basic functionality)

## Cross-Compilation

Build for different architectures from any Linux/macOS machine:

```bash
# AMD64 (most common)
GOOS=linux GOARCH=amd64 go build -o coretrace-agent-linux-amd64 .

# ARM64 (Raspberry Pi, Graviton, etc.)
GOOS=linux GOARCH=arm64 go build -o coretrace-agent-linux-arm64 .
```

## Verification

```bash
# Check version
./coretrace-agent --version

# Test help
./coretrace-agent monitor --help

# Run tests (if available)
go test ./...
```

## Deployment

The agent is a single binary. Deployment options:

1. **Direct install:**
   ```bash
   sudo cp coretrace-agent /usr/local/bin/
   sudo chmod +x /usr/local/bin/coretrace-agent
   ```

2. **Systemd service:**
   ```bash
   sudo cp coretrace-agent /usr/local/bin/
   sudo cp deploy/coretrace.service /etc/systemd/system/
   sudo systemctl enable coretrace
   sudo systemctl start coretrace
   ```

3. **Container:**
   ```bash
   docker run --privileged -v /var/log:/var/log:ro \
     coretrace/agent:latest
   ```

## Development

### Adding new monitoring backends

The command monitoring supports pluggable backends:

1. **Add new file** in `internal/monitor/command_<backend>.go`
2. **Implement interface:**
   - `Start(ctx context.Context) error`
   - `Stop() error`
   - `RegisterSession(sessionID string, pid int)`
   - `UnregisterSession(sessionID string)`
   - `Cleanup()`
3. **Update** `internal/monitor/command.go` to use new backend

### Future: eBPF support

When adding eBPF support:

```bash
# Install dependencies
sudo apt-get install clang llvm libbpf-dev

# Install Go eBPF library
go get github.com/cilium/ebpf

# Generate eBPF code (generates .go from .c)
go generate ./internal/ebpf/...

# Build with eBPF support
go build -tags linux -o coretrace-agent .
```

## Troubleshooting

### "undefined: syscall.Sysinfo"
This is expected - some syscall types differ between architectures. The code handles this with build tags.

### Large binary size
If the binary is too large, strip debug symbols:
```bash
go build -ldflags="-s -w" -o coretrace-agent .
```

This reduces size from ~15MB to ~10MB.
