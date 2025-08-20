# Network Resilience Improvements for Mac2MQTT

## Overview
These improvements make Mac2MQTT much more resilient when switching between networks, especially when moving between networks where the MQTT broker is reachable vs. unreachable.

## Key Improvements

### 1. Network Reachability Checking
- **Fast network tests**: 5-second timeout checks before attempting MQTT connections
- **Prevents long hangs**: Avoids 30+ second connection timeouts on unreachable networks
- **Smart logging**: Different messages for network vs. connection issues

### 2. Graceful Offline Mode
- **Continues running**: Application doesn't crash when broker is unreachable
- **Periodic reconnection**: Automatically attempts to reconnect when network becomes available
- **State preservation**: Maintains system monitoring even when offline

### 3. Network Change Detection
- **30-second network checks**: Monitors broker reachability every 30 seconds
- **State change logging**: Clear messages when network/connection state changes
- **Automatic recovery**: Triggers reconnection attempts when network returns

### 4. Optimized Connection Settings
- **Faster timeouts**: 15s connect timeout, 10s ping timeout for quicker network change detection
- **Shorter retry intervals**: 15s between retries, 2min max interval for faster recovery
- **Better error handling**: Distinguishes between network and broker issues

### 5. Reconnection Intelligence
- **Network-aware reconnection**: Only attempts reconnection when network is reachable
- **Automatic setup**: Re-establishes device configuration and subscriptions on reconnect
- **State synchronization**: Sends fresh state updates after reconnection

## Network Scenarios Handled

### Scenario 1: Home → Office (Broker Available → Unavailable)
1. Application detects network change
2. MQTT connection fails
3. Switches to offline mode
4. Continues system monitoring
5. Logs network unavailability

### Scenario 2: Office → Home (Broker Unavailable → Available)
1. Periodic network check detects broker is reachable
2. Triggers reconnection attempt
3. Re-establishes MQTT connection
4. Sends device configuration
5. Synchronizes current state

### Scenario 3: Temporary Network Issues
1. Short network outages are handled by auto-reconnect
2. Longer outages trigger offline mode
3. Quick recovery when network returns

### Scenario 4: VPN Changes
1. Detects when broker becomes reachable/unreachable via VPN
2. Handles IP address changes gracefully
3. Reconnects automatically when VPN establishes connection

## Benefits for Mobile Users

- **No application crashes** when switching networks
- **Fast recovery** when returning to networks with MQTT access
- **Reduced battery drain** by avoiding aggressive reconnection attempts on unreachable networks
- **Clear status logging** to understand connection state
- **Seamless operation** - works whether broker is reachable or not

## Configuration Recommendations

For best results when frequently switching networks:

```yaml
# In mac2mqtt.yaml - use IP address if possible for faster resolution
mqtt_ip: 192.168.1.250  # Better than hostname for network switching
mqtt_port: 1883
```

## Monitoring Network State

The application now logs network state changes:
- "Network connectivity restored - MQTT broker is now reachable"
- "Network connectivity lost - MQTT broker is no longer reachable"  
- "MQTT connection restored"
- "MQTT connection lost"
- "Operating in offline mode - MQTT broker not reachable"

These logs help you understand what's happening when you switch networks.