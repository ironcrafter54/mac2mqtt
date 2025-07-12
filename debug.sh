#!/bin/bash

# Mac2MQTT Debug Script
# This script helps debug issues with the Mac2MQTT background service

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Get current user
USER=$(whoami)
PLIST_PATH="/Users/$USER/Library/LaunchAgents/com.hagak.mac2mqtt.plist"
INSTALL_DIR="/Users/$USER/mac2mqtt"

echo "=== MAC2MQTT DEBUG REPORT ==="
echo "Date: $(date)"
echo "User: $USER"
echo ""

# Check if service is running
print_status "Checking service status..."
if launchctl list | grep -q "com.hagak.mac2mqtt"; then
    print_success "Service is running"
else
    print_error "Service is NOT running"
fi
echo ""

# Check plist file
print_status "Checking launch agent configuration..."
if [ -f "$PLIST_PATH" ]; then
    print_success "Launch agent plist exists"
    echo "Plist contents:"
    cat "$PLIST_PATH"
    echo ""
else
    print_error "Launch agent plist not found at: $PLIST_PATH"
fi

# Check installation directory
print_status "Checking installation directory..."
if [ -d "$INSTALL_DIR" ]; then
    print_success "Installation directory exists: $INSTALL_DIR"
    echo "Files in installation directory:"
    ls -la "$INSTALL_DIR"
    echo ""
else
    print_error "Installation directory not found: $INSTALL_DIR"
fi

# Check executable
print_status "Checking executable..."
if [ -f "$INSTALL_DIR/mac2mqtt" ]; then
    print_success "Executable exists"
    if [ -x "$INSTALL_DIR/mac2mqtt" ]; then
        print_success "Executable is executable"
    else
        print_error "Executable is not executable"
    fi
else
    print_error "Executable not found: $INSTALL_DIR/mac2mqtt"
fi

# Check config file
print_status "Checking configuration file..."
if [ -f "$INSTALL_DIR/mac2mqtt.yaml" ]; then
    print_success "Configuration file exists"
    echo "Configuration contents:"
    cat "$INSTALL_DIR/mac2mqtt.yaml"
    echo ""
else
    print_error "Configuration file not found: $INSTALL_DIR/mac2mqtt.yaml"
fi

# Check log files
print_status "Checking log files..."
if [ -f "/tmp/mac2mqtt.job.out" ]; then
    print_success "Standard output log exists"
    echo "=== Recent Standard Output ==="
    tail -n 20 "/tmp/mac2mqtt.job.out"
    echo ""
else
    print_warning "Standard output log not found"
fi

if [ -f "/tmp/mac2mqtt.job.err" ]; then
    print_success "Error log exists"
    echo "=== Recent Error Log ==="
    tail -n 20 "/tmp/mac2mqtt.job.err"
    echo ""
else
    print_warning "Error log not found"
fi

# Check environment
print_status "Checking environment..."
echo "Current PATH: $PATH"
echo "Current working directory: $(pwd)"
echo ""

# Check dependencies
print_status "Checking dependencies..."
echo "BetterDisplay CLI:"
if command -v betterdisplaycli >/dev/null 2>&1; then
    print_success "BetterDisplay CLI is available"
else
    print_warning "BetterDisplay CLI not found in PATH"
fi

echo "Media Control:"
if command -v media-control >/dev/null 2>&1; then
    print_success "Media Control is available"
else
    print_warning "Media Control not found in PATH"
fi

echo "Switch Audio Source:"
if command -v switchaudiosource >/dev/null 2>&1; then
    print_success "Switch Audio Source is available"
else
    print_warning "Switch Audio Source not found in PATH"
fi

# Test manual execution
print_status "Testing manual execution..."
if [ -f "$INSTALL_DIR/mac2mqtt" ]; then
    echo "Testing executable from installation directory..."
    cd "$INSTALL_DIR"
    timeout 10s ./mac2mqtt 2>&1 | head -n 10 || echo "Manual execution test completed"
    cd - > /dev/null
    echo ""
fi

# Check network connectivity
print_status "Checking network connectivity..."
if [ -f "$INSTALL_DIR/mac2mqtt.yaml" ]; then
    MQTT_IP=$(grep "mqtt_ip:" "$INSTALL_DIR/mac2mqtt.yaml" | cut -d' ' -f2)
    MQTT_PORT=$(grep "mqtt_port:" "$INSTALL_DIR/mac2mqtt.yaml" | cut -d' ' -f2)
    
    if [ -n "$MQTT_IP" ] && [ -n "$MQTT_PORT" ]; then
        echo "Testing connection to MQTT broker: $MQTT_IP:$MQTT_PORT"
        if nc -z "$MQTT_IP" "$MQTT_PORT" 2>/dev/null; then
            print_success "MQTT broker is reachable"
        else
            print_error "MQTT broker is NOT reachable"
        fi
    else
        print_warning "Could not extract MQTT broker info from config"
    fi
fi

echo ""
echo "=== DEBUG REPORT COMPLETE ==="
echo ""
print_status "Recommendations:"
echo "1. If service is not running, try: ./restart.sh"
echo "2. If logs show errors, check the error messages above"
echo "3. If dependencies are missing, install them:"
echo "   - BetterDisplay: https://github.com/waydabber/BetterDisplay"
echo "   - Media Control: npm install -g media-control"
echo "   - Switch Audio Source: brew install switchaudio-osx"
echo "4. If network issues, check your MQTT broker configuration"
echo "5. To monitor logs in real-time: ./status.sh --follow" 