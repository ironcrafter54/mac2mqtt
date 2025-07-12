#!/bin/bash

# Mac2MQTT Status Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

# Check if service is running
check_service() {
    if launchctl list | grep -q "com.hagak.mac2mqtt"; then
        print_success "Mac2MQTT service is running"
        return 0
    else
        print_error "Mac2MQTT service is not running"
        return 1
    fi
}

# Show logs
show_logs() {
    print_status "Recent logs:"
    if [ -f "/tmp/mac2mqtt.job.out" ]; then
        echo "=== Standard Output ==="
        tail -n 20 "/tmp/mac2mqtt.job.out"
    fi
    if [ -f "/tmp/mac2mqtt.job.err" ]; then
        echo "=== Error Log ==="
        tail -n 20 "/tmp/mac2mqtt.job.err"
    fi
}

# Follow logs
follow_logs() {
    print_status "Following logs (Ctrl+C to stop):"
    tail -f "/tmp/mac2mqtt.job.out" "/tmp/mac2mqtt.job.err" 2>/dev/null || print_warning "No log files found"
}

# Main logic
case "${1:-}" in
    --logs)
        show_logs
        ;;
    --follow)
        follow_logs
        ;;
    *)
        echo "Mac2MQTT Status Check"
        echo "===================="
        check_service
        echo ""
        show_logs
        ;;
esac 