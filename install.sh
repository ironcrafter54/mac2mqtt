#!/bin/bash

# Mac2MQTT Installer Script
# This script installs Mac2MQTT on macOS

set -e  # Exit on any error

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

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to get current user
get_current_user() {
    whoami
}

# Function to check if running as root
check_root() {
    if [[ $EUID -eq 0 ]]; then
        print_error "This script should not be run as root. Please run as a regular user."
        exit 1
    fi
}

# Function to check macOS
check_macos() {
    if [[ "$OSTYPE" != "darwin"* ]]; then
        print_error "This script is designed for macOS only."
        exit 1
    fi
}

# Function to check Go installation
check_go() {
    if ! command_exists go; then
        print_warning "Go is not installed. Attempting to install via Homebrew..."
        if command_exists brew; then
            brew install go
            print_success "Go installed successfully"
        else
            print_error "Homebrew not found. Please install Go manually from https://golang.org/dl/"
            exit 1
        fi
    else
        print_success "Go is already installed"
    fi
}

# Function to build the application
build_application() {
    print_status "Building Mac2MQTT..."
    
    if [ ! -f "mac2mqtt.go" ]; then
        print_error "mac2mqtt.go not found in current directory"
        exit 1
    fi
    
    # Download dependencies
    go mod download
    
    # Build the application
    go build -o mac2mqtt mac2mqtt.go
    
    # Make executable
    chmod +x mac2mqtt
    
    print_success "Mac2MQTT built successfully"
}

# Function to configure MQTT settings
configure_mqtt() {
    print_status "Configuring MQTT settings..."
    
    # Check if config file exists
    if [ -f "mac2mqtt.yaml" ]; then
        print_warning "mac2mqtt.yaml already exists. Do you want to overwrite it? (y/N)"
        read -r response
        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            print_status "Keeping existing configuration"
            return
        fi
    fi
    
    # Get MQTT configuration from user
    echo ""
    print_status "Please provide your MQTT configuration:"
    
    read -p "MQTT Broker IP/Hostname [192.168.1.250]: " mqtt_ip
    mqtt_ip=${mqtt_ip:-192.168.1.250}
    
    read -p "MQTT Port [1883]: " mqtt_port
    mqtt_port=${mqtt_port:-1883}
    
    read -p "MQTT Username [hass]: " mqtt_user
    mqtt_user=${mqtt_user:-hass}
    
    read -s -p "MQTT Password: " mqtt_password
    echo ""
    
    read -p "Use SSL/TLS? (y/N): " mqtt_ssl_response
    if [[ "$mqtt_ssl_response" =~ ^[Yy]$ ]]; then
        mqtt_ssl="true"
    else
        mqtt_ssl="false"
    fi
    
    read -p "Computer Hostname [$(hostname)]: " hostname
    hostname=${hostname:-$(hostname)}
    
    read -p "MQTT Topic Prefix [iot/MyMac]: " mqtt_topic
    mqtt_topic=${mqtt_topic:-iot/MyMac}
    
    # Create configuration file
    cat > mac2mqtt.yaml << EOF
mqtt_ip: $mqtt_ip
mqtt_port: $mqtt_port
mqtt_user: $mqtt_user
mqtt_password: $mqtt_password
mqtt_ssl: $mqtt_ssl
hostname: $hostname
mqtt_topic: $mqtt_topic
EOF
    
    print_success "MQTT configuration saved to mac2mqtt.yaml"
}

# Function to install optional dependencies
install_optional_deps() {
    print_status "Checking optional dependencies..."
    
    # Check for BetterDisplay CLI
    if ! command_exists betterdisplay; then
        print_warning "BetterDisplay CLI not found. This is required for display brightness control."
        print_status "Install BetterDisplay from https://github.com/waydabber/BetterDisplay"
        print_status "Then enable CLI access in BetterDisplay settings"
    else
        print_success "BetterDisplay CLI is available"
    fi
    
    # Check for Media Control
    if ! command_exists media-control; then
        print_warning "Media Control not found. This is required for media player information."
        print_status "Installing Media Control..."
        if command_exists npm; then
            npm install -g media-control
            print_success "Media Control installed via npm"
        elif command_exists brew; then
            brew install media-control
            print_success "Media Control installed via Homebrew"
        else
            print_warning "Neither npm nor Homebrew found. Please install Media Control manually:"
            print_status "  npm install -g media-control"
            print_status "  or"
            print_status "  brew install media-control"
        fi
    else
        print_success "Media Control is available"
    fi
}

# Function to create installation directory
create_install_dir() {
    local user=$(get_current_user)
    local install_dir="/Users/$user/mac2mqtt"
    
    print_status "Creating installation directory: $install_dir"
    
    mkdir -p "$install_dir"
    
    # Copy files to installation directory
    cp mac2mqtt "$install_dir/"
    cp mac2mqtt.yaml "$install_dir/"
    
    # Copy management scripts if they exist
    if [ -f "restart.sh" ]; then
        cp restart.sh "$install_dir/"
        chmod +x "$install_dir/restart.sh"
    fi
    if [ -f "debug.sh" ]; then
        cp debug.sh "$install_dir/"
        chmod +x "$install_dir/debug.sh"
    fi
    
    print_success "Files copied to installation directory"
}

# Function to setup launch agent
setup_launch_agent() {
    local user=$(get_current_user)
    local install_dir="/Users/$user/mac2mqtt"
    
    print_status "Setting up launch agent..."
    
    # Create plist file with correct username
    sed "s/USERNAME/$user/g" com.hagak.mac2mqtt.plist > "/tmp/com.hagak.mac2mqtt.plist"
    
    # Create user's LaunchAgents directory if it doesn't exist
    mkdir -p "/Users/$user/Library/LaunchAgents"
    
    # Copy to user's LaunchAgents directory (no root required)
    cp "/tmp/com.hagak.mac2mqtt.plist" "/Users/$user/Library/LaunchAgents/"
    
    # Set proper permissions
    chmod 644 "/Users/$user/Library/LaunchAgents/com.hagak.mac2mqtt.plist"
    
    # Load the launch agent
    launchctl load "/Users/$user/Library/LaunchAgents/com.hagak.mac2mqtt.plist"
    
    # Clean up temp file
    rm "/tmp/com.hagak.mac2mqtt.plist"
    
    print_success "Launch agent installed and loaded"
}

# Function to create management scripts
create_management_scripts() {
    local user=$(get_current_user)
    local install_dir="/Users/$user/mac2mqtt"
    
    print_status "Creating management scripts..."
    
    # Status script
    cat > "$install_dir/status.sh" << 'EOF'
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
EOF

    # Configure script
    cat > "$install_dir/configure.sh" << 'EOF'
#!/bin/bash

# Mac2MQTT Configuration Script

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

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Stop service
print_status "Stopping Mac2MQTT service..."
launchctl unload "/Users/$(whoami)/Library/LaunchAgents/com.hagak.mac2mqtt.plist" 2>/dev/null || true

# Get current configuration
if [ -f "mac2mqtt.yaml" ]; then
    print_status "Current configuration:"
    cat mac2mqtt.yaml
    echo ""
fi

# Get new MQTT configuration
print_status "Please provide new MQTT configuration:"

read -p "MQTT Broker IP/Hostname [192.168.1.250]: " mqtt_ip
mqtt_ip=${mqtt_ip:-192.168.1.250}

read -p "MQTT Port [1883]: " mqtt_port
mqtt_port=${mqtt_port:-1883}

read -p "MQTT Username [hass]: " mqtt_user
mqtt_user=${mqtt_user:-hass}

read -s -p "MQTT Password: " mqtt_password
echo ""

read -p "Use SSL/TLS? (y/N): " mqtt_ssl_response
if [[ "$mqtt_ssl_response" =~ ^[Yy]$ ]]; then
    mqtt_ssl="true"
else
    mqtt_ssl="false"
fi

read -p "Computer Hostname [$(hostname)]: " hostname
hostname=${hostname:-$(hostname)}

read -p "MQTT Topic Prefix [iot/MyMac]: " mqtt_topic
mqtt_topic=${mqtt_topic:-iot/MyMac}

# Update configuration file
cat > mac2mqtt.yaml << EOF
mqtt_ip: $mqtt_ip
mqtt_port: $mqtt_port
mqtt_user: $mqtt_user
mqtt_password: $mqtt_password
mqtt_ssl: $mqtt_ssl
hostname: $hostname
mqtt_topic: $mqtt_topic
EOF

print_success "Configuration updated"

# Restart service
print_status "Restarting Mac2MQTT service..."
launchctl load "/Users/$(whoami)/Library/LaunchAgents/com.hagak.mac2mqtt.plist"

print_success "Configuration complete!"
EOF

    # Uninstall script
    cat > "$install_dir/uninstall.sh" << 'EOF'
#!/bin/bash

# Mac2MQTT Uninstall Script

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

# Confirm uninstall
print_warning "This will completely remove Mac2MQTT from your system."
read -p "Are you sure you want to continue? (y/N): " response
if [[ ! "$response" =~ ^[Yy]$ ]]; then
    print_status "Uninstall cancelled"
    exit 0
fi

# Stop and unload service
print_status "Stopping Mac2MQTT service..."
launchctl unload "/Users/$(whoami)/Library/LaunchAgents/com.hagak.mac2mqtt.plist" 2>/dev/null || true

# Remove launch agent
print_status "Removing launch agent..."
rm -f "/Users/$(whoami)/Library/LaunchAgents/com.hagak.mac2mqtt.plist"

# Remove installation directory
print_status "Removing installation files..."
rm -rf "/Users/$(whoami)/mac2mqtt"

# Remove log files
print_status "Cleaning up log files..."
rm -f "/tmp/mac2mqtt.job.out" "/tmp/mac2mqtt.job.err"

print_success "Mac2MQTT has been completely removed from your system"
EOF

    # Make scripts executable
    chmod +x "$install_dir"/*.sh
    
    print_success "Management scripts created"
}

# Function to test installation
test_installation() {
    print_status "Testing installation..."
    
    # Wait a moment for service to start
    sleep 2
    
    # Check if service is running
    if launchctl list | grep -q "com.hagak.mac2mqtt"; then
        print_success "Mac2MQTT service is running"
    else
        print_warning "Mac2MQTT service may not be running. Check logs with: ./status.sh"
    fi
    
    # Check if log files exist
    if [ -f "/tmp/mac2mqtt.job.out" ]; then
        print_success "Log files are being created"
    else
        print_warning "No log files found yet. Service may still be starting."
    fi
}

# Function to show post-install instructions
show_post_install() {
    local user=$(get_current_user)
    local install_dir="/Users/$user/mac2mqtt"
    
    echo ""
    echo "=========================================="
    echo "Mac2MQTT Installation Complete!"
    echo "=========================================="
    echo ""
    echo "Installation directory: $install_dir"
    echo ""
    echo "Management commands:"
    echo "  cd $install_dir"
    echo "  ./status.sh          # Check service status"
    echo "  ./status.sh --logs   # Show recent logs"
    echo "  ./status.sh --follow # Follow logs in real-time"
    echo "  ./configure.sh       # Reconfigure MQTT settings"
    echo "  ./uninstall.sh       # Uninstall Mac2MQTT"
    echo ""
    echo "Or use the Makefile from the project directory:"
    echo "  make status          # Check service status"
    echo "  make logs            # Show recent logs"
    echo "  make configure       # Reconfigure settings"
    echo "  make uninstall       # Uninstall mac2mqtt"
    echo ""
    echo "MQTT Topics:"
    echo "  Status: mac2mqtt/$(hostname)/status/#"
    echo "  Commands: mac2mqtt/$(hostname)/command/#"
    echo ""
    echo "For Home Assistant integration, enable MQTT autodiscovery"
    echo "or see the README.md for manual configuration examples."
    echo ""
}

# Main installation function
main() {
    echo "=========================================="
    echo "Mac2MQTT Installer"
    echo "=========================================="
    echo ""
    
    # Pre-installation checks
    check_root
    check_macos
    check_go
    
    # Installation steps
    build_application
    configure_mqtt
    install_optional_deps
    create_install_dir
    setup_launch_agent
    create_management_scripts
    test_installation
    show_post_install
    
    print_success "Installation completed successfully!"
}

# Run main function
main "$@" 