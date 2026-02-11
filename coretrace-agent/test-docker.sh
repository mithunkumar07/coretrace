#!/bin/bash

# Test script for CoreTrace Agent Docker image

set -e

echo "🧪 Testing CoreTrace Agent Docker Image..."

# Test 1: Check if image exists
echo "📦 Checking if Docker image exists..."
if docker images coretrace/coretrace-agent:dev | grep -q "coretrace/coretrace-agent"; then
    echo "✅ Docker image exists"
else
    echo "❌ Docker image not found"
    exit 1
fi

# Test 2: Check if binary is executable
echo "🔍 Testing binary execution..."
if docker run --rm coretrace/coretrace-agent:dev --help > /dev/null 2>&1; then
    echo "✅ Binary is executable"
else
    echo "❌ Binary execution failed"
    exit 1
fi

# Test 3: Check configuration file
echo "📋 Testing configuration file..."
if docker run --rm --entrypoint cat coretrace/coretrace-agent:dev /etc/coretrace/config.yaml > /dev/null 2>&1; then
    echo "✅ Configuration file exists"
else
    echo "❌ Configuration file not found"
    exit 1
fi

# Test 4: Check user permissions
echo "👤 Testing user permissions..."
USER_CHECK=$(docker run --rm --entrypoint whoami coretrace/coretrace-agent:dev)
if [ "$USER_CHECK" = "coretrace" ]; then
    echo "✅ Running as non-root user"
else
    echo "❌ Running as root user (security issue)"
    exit 1
fi

# Test 5: Check image size
echo "📏 Checking image size..."
IMAGE_SIZE=$(docker images coretrace/coretrace-agent:dev --format "{{.Size}}" | head -n1)
echo "📊 Image size: $IMAGE_SIZE"

# Test 6: Check dependencies
echo "🔧 Testing dependencies..."
DEPS_CHECK=$(docker run --rm --entrypoint ls coretrace/coretrace-agent:dev -la /usr/local/bin/coretrace-agent)
if echo "$DEPS_CHECK" | grep -q "coretrace-agent"; then
    echo "✅ Binary exists in correct location"
else
    echo "❌ Binary not found"
    exit 1
fi

echo ""
echo "🎉 All tests passed! The Docker image is working correctly."
echo ""
echo "🚀 To run the agent:"
echo "   docker run --privileged -v /var/log:/var/log:ro coretrace/coretrace-agent:dev"
echo ""
echo "🔧 To run with custom config:"
echo "   docker run --privileged -v /var/log:/var/log:ro -v \$(pwd)/config.yaml:/etc/coretrace/config.yaml:ro coretrace/coretrace-agent:dev"
echo ""
echo "🐛 To run in debug mode:"
echo "   docker run --privileged -v /var/log:/var/log:ro coretrace/coretrace-agent:dev --debug"