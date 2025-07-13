# System Commands Fix - Using /command/set

## Problem
The system commands (displaysleep, screensaver, sleep, displaywake, shutdown) were not working because the autodiscovery configuration was publishing commands to individual topics, but the application was listening for commands on `/command/set`.

## Root Cause
The autodiscovery configuration was setting up individual command topics for each button:
- `/command/displaysleep`
- `/command/screensaver` 
- `/command/sleep`
- `/command/displaywake`
- `/command/shutdown`

But the `handleSystemCommand` function was only listening for commands on `/command/set` and expecting the command type in the payload.

## Solution
Updated the autodiscovery configuration to use `/command/set` for all system commands, with the command type specified in the payload.

## Changes Made

### Updated Autodiscovery Configuration
Changed all system command buttons to use `/command/set`:

```go
displaywake := map[string]interface{}{
    "p":             "button",
    "name":          "Display Wake",
    "unique_id":     app.hostname + "_displaywake",
    "command_topic": app.getTopicPrefix() + "/command/set",  // Changed from /command/displaywake
    "payload_press": "displaywake",
    "icon":          "mdi:monitor",
}

displaysleep := map[string]interface{}{
    "p":             "button",
    "name":          "Display Sleep",
    "unique_id":     app.hostname + "_displaysleep",
    "command_topic": app.getTopicPrefix() + "/command/set",  // Changed from /command/displaysleep
    "payload_press": "displaysleep",
    "icon":          "mdi:monitor-off",
}

screensaver := map[string]interface{}{
    "p":             "button",
    "name":          "Screensaver",
    "unique_id":     app.hostname + "_screensaver",
    "command_topic": app.getTopicPrefix() + "/command/set",  // Changed from /command/screensaver
    "payload_press": "screensaver",
    "icon":          "mdi:monitor-star",
}

sleep := map[string]interface{}{
    "p":             "button",
    "name":          "Sleep",
    "unique_id":     app.hostname + "_sleep",
    "command_topic": app.getTopicPrefix() + "/command/set",  // Changed from /command/sleep
    "payload_press": "sleep",
    "icon":          "mdi:sleep",
}

shutdown := map[string]interface{}{
    "p":                  "button",
    "name":               "Shutdown",
    "unique_id":          app.hostname + "_shutdown",
    "command_topic":      app.getTopicPrefix() + "/command/set",  // Changed from /command/shutdown
    "payload_press":      "shutdown",
    "enabled_by_default": false,
    "icon":               "mdi:power",
}
```

## How It Works Now

### MQTT Topics
All system commands now use the same topic: `mac2mqtt/MyMac/command/set`

### Payloads
- Display Sleep: `displaysleep`
- Screensaver: `screensaver`
- Display Wake: `displaywake`
- Sleep: `sleep`
- Shutdown: `shutdown`

### Command Handling
The existing `handleSystemCommand` function already handles these payloads correctly:

```go
func (app *Application) handleSystemCommand(topic, payload string) bool {
    if topic != app.getTopicPrefix()+"/command/set" {
        return false
    }

    switch payload {
    case "sleep":
        commandSleep()
    case "displaysleep":
        commandDisplaySleep()
    case "displaywake":
        commandDisplayWake()
    case "shutdown":
        commandShutdown()
    case "screensaver":
        commandScreensaver()
    default:
        log.Printf("Unknown system command: %s", payload)
    }
    return true
}
```

## Testing
Use the provided test script to verify the commands work:

```bash
./test_set_commands.sh
```

The script will publish test messages to `/command/set` with the appropriate payloads.

## Commands Now Working
- ✅ Display Sleep (`/command/set` with payload `displaysleep`)
- ✅ Screensaver (`/command/set` with payload `screensaver`)
- ✅ Display Wake (`/command/set` with payload `displaywake`)
- ✅ Sleep (`/command/set` with payload `sleep`)
- ✅ Shutdown (`/command/set` with payload `shutdown`)

## Benefits
- **Consistent**: All system commands use the same topic
- **Simple**: No need for multiple command handlers
- **Maintainable**: Easy to add new system commands
- **Compatible**: Works with existing Home Assistant configurations 