# Creating a GitHub Release for CoreTrace Agent

## Method 1: Using GitHub Web Interface (Easiest)

### Step 1: Build the Binary Locally

```bash
cd coretrace-agent

# Build for Linux AMD64
go build -o coretrace-agent .

# (Optional) Build for other architectures
go build -o coretrace-agent-arm64 .

# Strip debug symbols to reduce size
go build -ldflags="-s -w" -o coretrace-agent .
```

### Step 2: Push Code to GitHub

```bash
# Make sure all your changes are committed
git add .
git commit -m "Ready for v1.0.0 release"

# Push to main branch
git push origin main
```

### Step 3: Create Release on GitHub

1. Go to your repository: https://github.com/mithunkumar07/coretrace
2. Click on **"Releases"** (right sidebar)
3. Click **"Create a new release"** (green button)
4. Fill in the form:
   - **Choose a tag:** Type `v1.0.0` and select "Create new tag: v1.0.0"
   - **Release title:** `CoreTrace Agent v1.0.0`
   - **Description:** 
     ```
     ## CoreTrace Agent v1.0.0
     
     ### Features
     - SSH session monitoring with key fingerprinting
     - Real-time file integrity monitoring
     - Command logging via auditd
     - Session-based log rotation
     - Systemd service integration
     
     ### Installation
     ```bash
     curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/install.sh | sudo bash
     ```
     
     ### Binaries
     - `coretrace-agent` - Linux AMD64
     ```
5. **Attach binaries:**
   - Click "Attach binaries by dropping them here or selecting them"
   - Upload your `coretrace-agent` file
6. Click **"Publish release"**

### Step 4: Test the Install Script

```bash
# On a fresh server
curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/install.sh | sudo bash
```

---

## Method 2: Using GitHub CLI (Command Line)

### Install GitHub CLI

```bash
# macOS
brew install gh

# Ubuntu/Debian
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
sudo chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
sudo apt update
sudo apt install gh
```

### Authenticate

```bash
gh auth login
# Follow prompts to authenticate with browser
```

### Create Release

```bash
# Navigate to your repo
cd /path/to/coretrace

# Create and push tag
git tag -a v1.0.0 -m "CoreTrace Agent v1.0.0"
git push origin v1.0.0

# Build binary
cd coretrace-agent
go build -ldflags="-s -w" -o coretrace-agent .

# Create release with binary
gh release create v1.0.0 \
  --title "CoreTrace Agent v1.0.0" \
  --notes "CoreTrace Agent v1.0.0 - Initial Release" \
  coretrace-agent
```

---

## Method 3: Using GitHub Actions (Automated - Recommended)

### Step 1: Create Workflow File

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Build binaries
      run: |
        cd coretrace-agent
        # Linux AMD64
        GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o coretrace-agent-linux-amd64 .
        # Linux ARM64
        GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o coretrace-agent-linux-arm64 .
    
    - name: Create Release
      uses: softprops/action-gh-release@v1
      with:
        files: |
          coretrace-agent/coretrace-agent-linux-amd64
          coretrace-agent/coretrace-agent-linux-arm64
        generate_release_notes: true
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Step 2: Commit and Push Workflow

```bash
git add .github/workflows/release.yml
git commit -m "Add automated release workflow"
git push origin main
```

### Step 3: Create Release by Pushing Tag

```bash
# This triggers the workflow
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions will automatically build and create the release!

---

## Quick Start (Easiest Method)

**Right now, for testing:**

```bash
# 1. Build locally
cd coretrace-agent
go build -o coretrace-agent .

# 2. Push code
git add .
git commit -m "v1.0.0 release"
git push origin main

# 3. Go to https://github.com/mithunkumar07/coretrace/releases
# 4. Click "Create a new release"
# 5. Tag: v1.0.0
# 6. Title: CoreTrace Agent v1.0.0
# 7. Upload coretrace-agent file
# 8. Click "Publish release"
```

**Then test:**
```bash
ssh root@your-server
curl -sSL https://raw.githubusercontent.com/mithunkumar07/coretrace/main/coretrace-agent/install.sh | sudo bash
```

That's it! 🎉
