#!/bin/bash

# CoreTrace Stack Management Script
# Usage: ./stack.sh [start|stop|restart|status|logs] [profile]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default variables
COMPOSE_FILE="docker-compose.yml"
ENV_FILE=".env"
PROFILE=""
ENVIRONMENT="development"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if Docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker first."
        exit 1
    fi
}

# Function to check if docker-compose is available
check_compose() {
    if ! command -v docker-compose >/dev/null 2>&1 && ! docker compose version >/dev/null 2>&1; then
        print_error "docker-compose is not installed."
        exit 1
    fi
}

# Function to get the docker-compose command
get_compose_cmd() {
    if command -v docker-compose >/dev/null 2>&1; then
        echo "docker-compose"
    else
        echo "docker compose"
    fi
}

# Function to setup environment
setup_environment() {
    local profile=$1
    
    if [ ! -f "$ENV_FILE" ]; then
        if [ -f ".env.example" ]; then
            print_status "Creating .env from .env.example..."
            cp .env.example .env
            print_success ".env file created"
        else
            print_warning ".env file not found, using defaults"
        fi
    fi
    
    # Set environment-specific variables
    case "$profile" in
        "dev"|"development")
            export ENVIRONMENT=development
            export LOG_LEVEL=debug
            export DEBUG=true
            export VERSION=dev
            export DOCKER_TAG=dev
            ;;
        "test"|"testing")
            export ENVIRONMENT=testing
            export LOG_LEVEL=debug
            export DEBUG=true
            export TEST_MODE=true
            export VERSION=test
            export DOCKER_TAG=test
            ;;
        "prod"|"production")
            export ENVIRONMENT=production
            export LOG_LEVEL=info
            export DEBUG=false
            if [ -z "$VERSION" ]; then
                export VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "latest")
            fi
            export DOCKER_TAG=$VERSION
            ;;
        "full")
            export ENVIRONMENT=production
            export LOG_LEVEL=info
            export DEBUG=false
            export VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "latest")
            export DOCKER_TAG=$VERSION
            ;;
        *)
            export ENVIRONMENT=development
            export LOG_LEVEL=info
            export DEBUG=false
            ;;
    esac
    
    # Set common variables
    export BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    export COMMIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    export DOCKER_REGISTRY=${DOCKER_REGISTRY:-coretrace}
    export AGENT_NAME=${AGENT_NAME:-coretrace-agent}
}

# Function to start services
start_services() {
    local profile=$1
    print_status "Starting CoreTrace stack with profile: $profile"
    
    setup_environment "$profile"
    
    local compose_cmd=$(get_compose_cmd)
    local compose_args=""
    
    case "$profile" in
        "dev"|"development")
            compose_args="--profile dev"
            ;;
        "test"|"testing")
            compose_args="--profile testing"
            ;;
        "full")
            compose_args="--profile full"
            ;;
        "prod"|"production")
            if [ -f "docker-compose.prod.yml" ]; then
                compose_args="-f docker-compose.yml -f docker-compose.prod.yml"
            fi
            ;;
    esac
    
    print_status "Environment: $ENVIRONMENT"
    print_status "Version: $VERSION"
    print_status "Log Level: $LOG_LEVEL"
    
    # Create necessary directories
    mkdir -p test/logs test/config test/ssh-config
    
    $compose_cmd $compose_args up -d
    
    print_success "CoreTrace stack started successfully!"
    print_status "Use '$0 logs $profile' to view logs"
}

# Function to stop services
stop_services() {
    local profile=$1
    print_status "Stopping CoreTrace stack..."
    
    local compose_cmd=$(get_compose_cmd)
    local compose_args=""
    
    case "$profile" in
        "dev"|"development")
            compose_args="--profile dev"
            ;;
        "test"|"testing")
            compose_args="--profile testing"
            ;;
        "full")
            compose_args="--profile full"
            ;;
        "prod"|"production")
            if [ -f "docker-compose.prod.yml" ]; then
                compose_args="-f docker-compose.yml -f docker-compose.prod.yml"
            fi
            ;;
    esac
    
    $compose_cmd $compose_args down
    
    print_success "CoreTrace stack stopped successfully!"
}

# Function to restart services
restart_services() {
    local profile=$1
    print_status "Restarting CoreTrace stack with profile: $profile"
    stop_services "$profile"
    sleep 2
    start_services "$profile"
}

# Function to show status
show_status() {
    local profile=$1
    print_status "CoreTrace stack status:"
    
    local compose_cmd=$(get_compose_cmd)
    local compose_args=""
    
    case "$profile" in
        "dev"|"development")
            compose_args="--profile dev"
            ;;
        "test"|"testing")
            compose_args="--profile testing"
            ;;
        "full")
            compose_args="--profile full"
            ;;
        "prod"|"production")
            if [ -f "docker-compose.prod.yml" ]; then
                compose_args="-f docker-compose.yml -f docker-compose.prod.yml"
            fi
            ;;
    esac
    
    $compose_cmd $compose_args ps
}

# Function to show logs
show_logs() {
    local profile=$1
    local service=$2
    
    local compose_cmd=$(get_compose_cmd)
    local compose_args=""
    
    case "$profile" in
        "dev"|"development")
            compose_args="--profile dev"
            ;;
        "test"|"testing")
            compose_args="--profile testing"
            ;;
        "full")
            compose_args="--profile full"
            ;;
        "prod"|"production")
            if [ -f "docker-compose.prod.yml" ]; then
                compose_args="-f docker-compose.yml -f docker-compose.prod.yml"
            fi
            ;;
    esac
    
    if [ -n "$service" ]; then
        $compose_cmd $compose_args logs -f "$service"
    else
        $compose_cmd $compose_args logs -f
    fi
}

# Function to show usage
show_usage() {
    cat << EOF
CoreTrace Stack Management Script

USAGE:
    $0 [COMMAND] [PROFILE] [OPTIONS]

COMMANDS:
    start       Start the CoreTrace stack
    stop        Stop the CoreTrace stack
    restart     Restart the CoreTrace stack
    status      Show status of running services
    logs        Show logs from services

PROFILES:
    dev         Development environment with hot reload
    test        Testing environment with SSH test server
    full        Full stack with collector, Redis, and PostgreSQL
    prod        Production environment (default)

EXAMPLES:
    $0 start dev              # Start development environment
    $0 start test             # Start testing environment
    $0 start prod             # Start production environment
    $0 start                  # Start production environment (default)
    $0 restart dev             # Restart development environment
    $0 logs dev               # Show logs for development environment
    $0 logs dev coretrace-agent # Show logs for specific service
    $0 status                 # Show status of default environment

ENVIRONMENT VARIABLES:
    DOCKER_REGISTRY            Docker registry (default: coretrace)
    VERSION                   Version tag for images
    LOG_LEVEL                 Logging level (info, debug, warn, error)
    DEBUG                     Enable debug mode (true/false)

EOF
}

# Main script logic
main() {
    local command=$1
    local profile=$2
    local service=$3
    
    # Default profile
    if [ -z "$profile" ]; then
        profile="prod"
    fi
    
    # Normalize profile names
    case "$profile" in
        "dev"|"development") profile="dev" ;;
        "test"|"testing") profile="test" ;;
        "prod"|"production") profile="prod" ;;
        "full") profile="full" ;;
        *) ;;
    esac
    
    check_docker
    check_compose
    
    case "$command" in
        "start")
            start_services "$profile"
            ;;
        "stop")
            stop_services "$profile"
            ;;
        "restart")
            restart_services "$profile"
            ;;
        "status")
            show_status "$profile"
            ;;
        "logs")
            show_logs "$profile" "$service"
            ;;
        "--help"|"-h"|"help")
            show_usage
            ;;
        *)
            print_error "Unknown command: $command"
            show_usage
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"