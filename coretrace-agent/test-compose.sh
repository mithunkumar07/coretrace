#!/bin/bash

# Test CoreTrace Docker Compose Setup

set -e

echo "🧪 Testing Docker Compose Configuration..."

# Test 1: Validate basic configuration
echo "🔧 Validating basic configuration..."
if docker-compose config --quiet > /dev/null 2>&1; then
    echo "✅ Basic configuration is valid"
else
    echo "❌ Basic configuration has errors"
    exit 1
fi

# Test 2: Validate dev profile
echo "🔧 Validating development profile..."
if docker-compose --profile dev config --quiet > /dev/null 2>&1; then
    echo "✅ Development profile is valid"
else
    echo "❌ Development profile has errors"
    exit 1
fi

# Test 3: Validate testing profile
echo "🔧 Validating testing profile..."
if docker-compose --profile testing config --quiet > /dev/null 2>&1; then
    echo "✅ Testing profile is valid"
else
    echo "❌ Testing profile has errors"
    exit 1
fi

# Test 4: Check environment file
echo "📋 Checking environment file..."
if [ -f ".env.example" ]; then
    echo "✅ Environment example file exists"
else
    echo "❌ Environment example file missing"
    exit 1
fi

# Test 5: Check documentation
echo "📚 Checking documentation..."
if [ -f "docker-compose-guide.md" ]; then
    echo "✅ Docker Compose guide exists"
else
    echo "❌ Docker Compose guide missing"
    exit 1
fi

echo ""
echo "🎉 All Docker Compose configurations are valid!"
echo ""
echo "🚀 Quick start commands:"
echo "   cp .env.example .env"
echo "   docker-compose up -d                    # Production"
echo "   docker-compose --profile dev up -d       # Development"
echo "   docker-compose --profile testing up -d    # Testing"
echo "   docker-compose --profile full up -d      # Full Stack"
echo ""
echo "📖 For detailed usage, see: docker-compose-guide.md"