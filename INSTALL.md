# Mac2MQTT Installation Guide

This guide explains how to install Mac2MQTT on your macOS system.

## Quick Installation

The easiest way to install Mac2MQTT is using the provided installer script:

```bash
# Make sure you're in the project directory
./install.sh
```

The installer will:
1. Check system requirements (macOS, Go)
2. Build the Mac2MQTT binary
3. Configure MQTT settings interactively
4. Install optional dependencies (BetterDisplay CLI, Media Control)
5. Set up the service to run automatically
6. Create management scripts

## Prerequisites

- macOS (required)
- Go programming language (will be installed automatically if missing)
- MQTT broker (e.g., Home Assistant, Mosquitto)

## Optional Dependencies

For full functionality, consider installing:

### BetterDisplay CLI
Required for display brightness control:
1. Install BetterDisplay from https://github.com/waydabber/BetterDisplay
2. Enable CLI access in BetterDisplay settings

### Media Control
Required for media player information:
```bash
# Via npm
npm install -g media-control

# Or via Homebrew
brew install media-control
```

## Manual Installation

If you prefer to install manually:

1. **Build the application:**
   ```bash
   go mod download
   go build -o mac2mqtt mac2mqtt.go
   chmod +x mac2mqtt
   ```

2. **Configure MQTT settings:**
   Edit `mac2mqtt.yaml` with your MQTT broker details.

3. **Create installation directory:**
   ```bash
   mkdir -p ~/mac2mqtt
   cp mac2mqtt mac2mqtt.yaml ~/mac2mqtt/
   ```

4. **Set up launch agent:**
   ```bash
   # Edit the plist file to replace USERNAME with your username
   sed "s/USERNAME/$(whoami)/g" com.hagak.mac2mqtt.plist > /tmp/com.hagak.mac2mqtt.plist
   sudo cp /tmp/com.hagak.mac2mqtt.plist /Library/LaunchAgents/
   sudo chown root:wheel /Library/LaunchAgents/com.hagak.mac2mqtt.plist
   sudo chmod 644 /Library/LaunchAgents/com.hagak.mac2mqtt.plist
   launchctl load /Library/LaunchAgents/com.hagak.mac2mqtt.plist
   ```

## Management

After installation, you can manage Mac2MQTT using:

### Using the Makefile (from project directory)
```bash
make status       # Check service status
make logs         # Show recent logs
make configure    # Reconfigure settings
make uninstall    # Uninstall mac2mqtt
```

### Using management scripts (from installation directory)
```bash
cd ~/mac2mqtt
./status.sh          # Check service status
./status.sh --logs   # Show recent logs
./status.sh --follow # Follow logs in real-time
./configure.sh       # Reconfigure MQTT settings
./uninstall.sh       # Uninstall Mac2MQTT
```

## Troubleshooting

### Service not starting
Check the logs:
```bash
tail -f /tmp/mac2mqtt.job.out /tmp/mac2mqtt.job.err
```

### MQTT connection issues
1. Verify your MQTT broker is running
2. Check the configuration in `~/mac2mqtt/mac2mqtt.yaml`
3. Ensure network connectivity to the MQTT broker

### Permission issues
If you encounter permission issues, ensure:
1. You're not running as root
2. The launch agent has proper permissions
3. The installation directory is owned by your user

## Uninstallation

To completely remove Mac2MQTT:

```bash
# Using the uninstall script
./uninstall.sh

# Or using make
make uninstall
```

This will:
- Stop the service
- Remove the launch agent
- Delete installation files
- Clean up log files

## Support

For issues and questions:
1. Check the logs using `./status.sh --logs`
2. Review the README.md for configuration examples
3. Ensure all dependencies are properly installed 