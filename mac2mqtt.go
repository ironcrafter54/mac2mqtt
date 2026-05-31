// main - obigatory main comment for package to appease the linting gods
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/mem" // Using v3 for current versions
	"gopkg.in/yaml.v2"

	sigar "github.com/cloudfoundry/gosigar"
	mqtt "github.com/eclipse/paho.mqtt.golang"

	// Using my fork until #9 is resolved ( https://github.com/antonfisher/go-media-devices-state/pull/9 )
	mediadevices "github.com/antonfisher/go-media-devices-state"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// Constants for the application
const (
	DefaultDiscoveryPrefix = "homeassistant"
	DefaultTopicPrefix     = "mac2mqtt"
	UpdateInterval         = 60 * time.Second
	MaxVolume              = 100
	MinVolume              = 0
	MaxBrightness          = 100
	MinBrightness          = 0
	MaxRetryAttempts       = 1
)

// BetterDisplayCLIError represents an error when BetterDisplay CLI is not available
type BetterDisplayCLIError struct {
	message string
}

func (e *BetterDisplayCLIError) Error() string {
	return e.message
}

// MediaControlError represents an error when Media Control is not available
type MediaControlError struct {
	message string
}

func (e *MediaControlError) Error() string {
	return e.message
}

// isBetterDisplayCLIAvailable checks if BetterDisplay CLI is installed and accessible
func isBetterDisplayCLIAvailable() bool {
	_, err := exec.LookPath("betterdisplaycli")
	return err == nil
}

// isMediaControlAvailable checks if Media Control is installed and accessible
func isMediaControlAvailable() bool {
	_, err := exec.LookPath("media-control")
	return err == nil
}

// MediaInfo represents the current media playing information
type MediaInfo struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	AppName     string `json:"app_name"`
	AppBundleID string `json:"app_bundle_id"`
	State       string `json:"state"`    // "playing", "paused", "stopped"
	Duration    int    `json:"duration"` // in seconds
	Position    int    `json:"position"` // in seconds
}

// Display represents the the display information
type Display struct {
	UUID               string `json:"UUID"`
	AlphanumericSerial string `json:"alphanumericSerial"`
	DeviceType         string `json:"deviceType"`
	DisplayID          string `json:"displayID"`
	Model              string `json:"model"`
	Name               string `json:"name"`
	OriginalName       string `json:"originalName"`
	ProductName        string `json:"productName"`
	RegistryLocation   string `json:"registryLocation"`
	Serial             string `json:"serial"`
	TagID              string `json:"tagID"`
	Vendor             string `json:"vendor"`
	WeekOfManufacture  string `json:"weekOfManufacture"`
	YearOfManufacture  string `json:"yearOfManufacture"`
}

// Application holds the main application state
type Application struct {
	config            *config
	displays          []Display
	hostname          string
	topic             string
	client            mqtt.Client
	currentMediaState MediaInfo // persistent media state for streaming
	userActivityState string    // "active" or "inactive"
	activityMutex     sync.RWMutex
	activityTimer     *time.Timer
	lastCPU           sigar.Cpu // for CPU percentage calculation
	cpuMutex          sync.RWMutex
}

type config struct {
	IP               string `yaml:"mqtt_ip"`
	Port             string `yaml:"mqtt_port"`
	User             string `yaml:"mqtt_user"`
	Password         string `yaml:"mqtt_password"`
	SSL              bool   `yaml:"mqtt_ssl"`
	Hostname         string `yaml:"hostname"`
	Topic            string `yaml:"mqtt_topic"`
	DiscoveryPrefix  string `yaml:"discovery_prefix"`
	IdleActivityTime int    `yaml:"idle_activity_time"` // in seconds
}

func (c *config) getConfig() *config {

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	log.Printf("Path: %v", exPath)
	configContent, err := os.ReadFile(exPath + "/mac2mqtt.yaml")
	if err != nil {
		log.Fatal("No config file provided")
	}

	err = yaml.Unmarshal(configContent, c)
	if err != nil {
		log.Fatal("No data in config file")
	}

	if c.IP == "" {
		log.Fatal("Must specify mqtt_ip in mac2mqtt.yaml")
	}

	if c.IdleActivityTime == 0 {
		log.Println("No idle_activity_time specified in config, using default 10 seconds")

	}

	if c.Port == "" {
		log.Fatal("Must specify mqtt_port in mac2mqtt.yaml")
	}

	if c.Hostname == "" {
		c.Hostname = getHostname()
	}
	if c.DiscoveryPrefix == "" {
		c.DiscoveryPrefix = "homeassistant"
	}
	return c
}

// NewApplication creates and initializes a new Application instance
func NewApplication() (*Application, error) {
	app := &Application{}

	// Load configuration
	app.config = &config{}
	app.config.getConfig()

	// Set hostname
	if app.config.Hostname == "" {
		app.hostname = getHostname()
	} else {
		app.hostname = app.config.Hostname
	}

	// Set topic - append hostname to allow multiple instances
	if app.config.Topic == "" {
		app.topic = DefaultTopicPrefix + "/" + app.hostname
	} else {
		// Append hostname to the configured topic
		app.topic = app.config.Topic + "/" + app.hostname
	}

	// Validate configuration
	if err := app.validateConfig(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Initialize displays
	app.displays = getDisplays()

	// Initialize currentMediaState
	if isMediaControlAvailable() {
		mediaInfo, err := getMediaInfo()
		if err == nil && mediaInfo != nil {
			app.currentMediaState = *mediaInfo
		} else {
			app.currentMediaState = MediaInfo{State: "idle"}
		}
	} else {
		app.currentMediaState = MediaInfo{State: "idle"}
	}

	// Initialize user activity state
	app.userActivityState = "inactive"

	// Initialize CPU stats for percentage calculation
	if err := app.lastCPU.Get(); err != nil {
		log.Printf("Warning: Failed to initialize CPU stats: %v", err)
	}

	return app, nil
}

// validateConfig validates the application configuration
func (app *Application) validateConfig() error {
	if app.config.IP == "" {
		return fmt.Errorf("mqtt_ip is required")
	}
	if app.config.Port == "" {
		return fmt.Errorf("mqtt_port is required")
	}
	if app.config.DiscoveryPrefix == "" {
		app.config.DiscoveryPrefix = DefaultDiscoveryPrefix
	}
	return nil
}

// getTopicPrefix returns the topic prefix for this application
func (app *Application) getTopicPrefix() string {
	return app.topic
}

func getSerialnumber() string {

	cmd := "/usr/sbin/ioreg -l | /usr/bin/grep IOPlatformSerialNumber"
	output, err := exec.Command("/bin/sh", "-c", cmd).Output()

	if err != nil {
		log.Fatal(err)
	}
	outputStr := string(output)
	last := output[strings.LastIndex(outputStr, " ")+1:]
	lastStr := string(last)
	// remove all symbols, but [a-zA-Z0-9_-]
	reg, err := regexp.Compile("[^a-zA-Z0-9_-]+")
	if err != nil {
		log.Fatal(err)
	}
	lastStr = reg.ReplaceAllString(lastStr, "")

	return lastStr
}

func getModel() string {

	cmd := "/usr/sbin/system_profiler SPHardwareDataType |/usr/bin/grep Chip | /usr/bin/sed 's/\\(^.*: \\)\\(.*\\)/\\2/'"
	output, err := exec.Command("/bin/sh", "-c", cmd).Output()

	if err != nil {
		log.Fatal(err)
	}
	outputStr := string(output)
	outputStr = strings.TrimSuffix(outputStr, "\n")
	return outputStr
}

func getHostname() string {

	hostname, err := os.Hostname()

	if err != nil {
		log.Fatal(err)
	}

	// "name.local" => "name"
	firstPart := strings.Split(hostname, ".")[0]

	// remove all symbols, but [a-zA-Z0-9_-]
	reg, err := regexp.Compile("[^a-zA-Z0-9_-]+")
	if err != nil {
		log.Fatal(err)
	}
	firstPart = reg.ReplaceAllString(firstPart, "")

	return firstPart
}

func getWorkingDirectory() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return wd
}

func getCommandOutput(name string, arg ...string) string {
	cmd := exec.Command(name, arg...)
	stdout, err := cmd.Output()
	if err != nil {
		log.Println("error: " + err.Error())
		log.Println("output: " + string(stdout))
		log.Fatal(err)
	}
	stdoutStr := string(stdout)
	stdoutStr = strings.TrimSuffix(stdoutStr, "\n")

	return stdoutStr
}

func getCaffeinateStatus() bool {
	cmd := "/bin/ps ax | /usr/bin/grep caffeinate | /usr/bin/grep -v grep"
	output, err := exec.Command("/bin/sh", "-c", cmd).Output()
	//revive:disable-next-line
	if err != nil {
		//log.Fatal(err)
	}
	stdoutStr := string(output)
	stdoutStr = strings.TrimSuffix(stdoutStr, "\n")
	return stdoutStr != ""
}

func getMuteStatus() bool {
	log.Println("Getting mute status")
	output := getCommandOutput("/usr/bin/osascript", "-e", "output muted of (get volume settings)")
	b, err := strconv.ParseBool(output)
	//revive:disable-next-line
	if err != nil {
		// Continue to fallback method
	}
	if output == "missing value" {
		currentsource := getCommandOutput("/opt/homebrew/bin/switchaudiosource", "-c")
		var resp *http.Response
		var err error

		// URL encode the current source name to handle spaces and special characters
		encodedSource := strings.ReplaceAll(currentsource, " ", "%20")
		url := fmt.Sprintf("http://localhost:55777/get?name=%s&mute", encodedSource)
		resp, err = http.Get(url)
		if err != nil {
			log.Printf("Error getting mute status for %s: %v", currentsource, err)
			return false
		}
		if resp != nil {
			defer resp.Body.Close()
			output, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error getting mute status body for %s: %v", currentsource, err)
				return false
			}
			output = []byte(strings.TrimSuffix(string(output), "\n"))
			mute := string(output)
			log.Println("Mute Output: " + mute)
			b = mute == "on"
		}
	}
	return b
}

func getCurrentVolume() int {
	log.Println("Getting volume status")
	output := getCommandOutput("/usr/bin/osascript", "-e", "output volume of (get volume settings)")
	output = strings.TrimSuffix(output, "\n")
	i, err := strconv.Atoi(output)
	if err != nil {
		currentsource := getCommandOutput("/opt/homebrew/bin/switchaudiosource", "-c")
		var resp *http.Response
		var err error
		// URL encode the current source name to handle spaces and special characters
		encodedSource := strings.ReplaceAll(currentsource, " ", "%20")
		url := fmt.Sprintf("http://localhost:55777/get?name=%s&volume", encodedSource)
		resp, err = http.Get(url)
		if err != nil {
			log.Printf("Error getting volume status for %s: %v", currentsource, err)
			return 0
		}
		if resp != nil {
			defer resp.Body.Close()
			output, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error getting volume status body for %s: %v", currentsource, err)
				return 0
			}
			output = []byte(strings.TrimSuffix(string(output), "\n"))
			outputStr := string(output)
			log.Println("Vol Output: " + outputStr)
			f, err := strconv.ParseFloat(outputStr, 64)
			if err != nil {
				log.Printf("Error parsing volume value for %s: %v", currentsource, err)
				return 0
			}
			i = int(f * 100)
		}
	}
	return i
}

func runCommand(name string, arg ...string) {
	cmd := exec.Command(name, arg...)

	_, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
}

// from 0 to 100
func setVolume(i int) {
	//Test first if we can control the mute if not use betterdisplaycli
	test := getCommandOutput("/usr/bin/osascript", "-e", "output volume of (get volume settings)")
	if test == "missing value" {
		volumef := float64(i) / 100
		currentsource := getCommandOutput("/opt/homebrew/bin/switchaudiosource", "-c")
		// URL encode the current source name to handle spaces and special characters
		encodedSource := strings.ReplaceAll(currentsource, " ", "%20")
		url := fmt.Sprintf("http://localhost:55777/set?name=%s&volume=%f", encodedSource, volumef)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error setting volume for %s: %v", currentsource, err)
		} else if resp != nil {
			resp.Body.Close()
		}
	} else {
		runCommand("/usr/bin/osascript", "-e", "set volume output volume "+strconv.Itoa(i))
	}
}

// true - turn mute on
// false - turn mute off
func setMute(b bool) {
	//Test first if we can control the mute if not use betterdisplaycli
	test := getCommandOutput("/usr/bin/osascript", "-e", "output volume of (get volume settings)")
	if test == "missing value" {
		state := "off"
		if b {
			state = "on"
		}
		currentsource := getCommandOutput("/opt/homebrew/bin/switchaudiosource", "-c")
		// URL encode the current source name to handle spaces and special characters
		encodedSource := strings.ReplaceAll(currentsource, " ", "%20")
		url := fmt.Sprintf("http://localhost:55777/set?name=%s&mute=%s", encodedSource, state)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error setting mute for %s: %v", currentsource, err)
		} else if resp != nil {
			resp.Body.Close()
		}
	} else {
		runCommand("/usr/bin/osascript", "-e", "set volume output muted "+strconv.FormatBool(b))
	}

}

func commandSleep() {
	runCommand("pmset", "sleepnow")
}

func commandDisplaySleep() {
	runCommand("pmset", "displaysleepnow")
}

func commandShutdown() {

	if os.Getuid() == 0 {
		// if the program is run by root user we are doing the most powerfull shutdown - that always shuts down the computer
		runCommand("shutdown", "-h", "now")
	} else {
		// if the program is run by ordinary user we are trying to shutdown, but it may fail if the other user is logged in
		runCommand("/usr/bin/osascript", "-e", "tell app \"System Events\" to shut down")
	}

}

func commandDisplayWake() {
	runCommand("/usr/bin/caffeinate", "-u", "-t", "1")
}

func commandKeepAwake() {
	cmd := "/usr/bin/caffeinate -d &"
	err := exec.Command("/bin/sh", "-c", cmd).Start()
	if err != nil {
		log.Fatal(err)
	}
}

func commandAllowSleep() {
	cmd := "/bin/ps ax | /usr/bin/grep caffeinate | /usr/bin/grep -v grep | /usr/bin/awk '{print \"kill \"$1}'|sh"
	_, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if err != nil {
		log.Fatal(err)
	}
}

func commandRunShortcut(shortcut string) {
	runCommand("shortcuts", "run", shortcut)
}

func commandScreensaver() {
	runCommand("open", "-a", "ScreenSaverEngine")
}

func commandPlayPause() {
	runCommand("media-control", "toggle-play-pause")
}

// getDisplays retrieves all available displays using BetterDisplay CLI
func getDisplays() []Display {

	// Check if BetterDisplay CLI is available
	if !isBetterDisplayCLIAvailable() {
		log.Println("BetterDisplay CLI is not installed or not accessible")
		log.Println("To install BetterDisplay CLI:")
		log.Println("  1. Install BetterDisplay from https://github.com/waydabber/BetterDisplay")
		log.Println("  2. Enable CLI access in BetterDisplay preferences")
		log.Println("  3. Restart the application")
		log.Println("Display brightness controls will be disabled until BetterDisplay CLI is available")
		return nil
	}

	log.Println("Executing: betterdisplaycli get -identifiers")
	out, err := exec.Command("betterdisplaycli", "get", "-identifiers").Output()
	if err != nil {
		log.Printf("Error getting displays: %v", err)
		log.Println("BetterDisplay CLI is installed but failed to execute")
		log.Println("Make sure BetterDisplay is running and CLI access is enabled")
		return nil
	}

	log.Printf("BetterDisplay CLI output: %s", string(out))

	// BetterDisplay CLI returns comma-separated JSON objects, not an array
	// We need to wrap it in brackets to make it a valid JSON array
	jsonStr := "[" + string(out) + "]"

	var displays []Display
	err = json.Unmarshal([]byte(jsonStr), &displays)
	if err != nil {
		log.Printf("Error parsing display JSON: %v", err)
		log.Println("BetterDisplay CLI returned invalid JSON format")
		return nil
	}

	return displays
}

// isDisplayAvailable checks if a display is currently available
func isDisplayAvailable(displayID string) bool {
	// Get current display list to check if display is available
	displays := getDisplays()
	if displays == nil {
		return false
	}

	for _, display := range displays {
		if display.DisplayID == displayID {
			return true
		}
	}
	return false
}

// getDisplayBrightness gets the current brightness for a specific display
func getDisplayBrightness(displayID string) (int, error) {
	// First check if display is available to avoid unnecessary errors
	if !isDisplayAvailable(displayID) {
		return 0, fmt.Errorf("display %s is not currently available", displayID)
	}

	out, err := exec.Command("betterdisplaycli", "get", "-displayID="+displayID, "-brightness", "-value").Output()
	if err != nil {
		return 0, fmt.Errorf("error getting brightness for display %s: %v", displayID, err)
	}

	// Parse the brightness value (0.0-1.0) and convert to percentage
	brightnessStr := strings.TrimSpace(string(out))
	brightness, err := strconv.ParseFloat(brightnessStr, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing brightness value: %v", err)
	}

	return int(brightness * 100), nil
}

// setDisplayBrightness sets the brightness for a specific display
func setDisplayBrightness(displayID string, brightness int) error {
	cmd := exec.Command("betterdisplaycli", "set", "-displayID="+displayID, "-brightness="+strconv.Itoa(brightness)+"%")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error setting brightness for display %s: %v", displayID, err)
	}
	return nil
}

// getMediaInfo retrieves current media information using Media Control
func getMediaInfo() (*MediaInfo, error) {
	// Check if Media Control is available
	if !isMediaControlAvailable() {
		return nil, &MediaControlError{message: "Media Control is not installed or not accessible"}
	}

	// Get media information in JSON format
	out, err := exec.Command("media-control", "get").Output()
	if err != nil {
		return nil, fmt.Errorf("error getting media info: %v", err)
	}

	// Parse JSON output
	var mediaData map[string]interface{}
	if err := json.Unmarshal(out, &mediaData); err != nil {
		return nil, fmt.Errorf("error parsing media-control JSON output: %v", err)
	}

	// Check if media is playing
	playing, ok := mediaData["playing"].(bool)
	if !ok || !playing {
		return nil, nil // No media playing
	}

	// Extract media information
	mediaInfo := &MediaInfo{}

	// Get title
	if title, ok := mediaData["title"].(string); ok && title != "" {
		mediaInfo.Title = title
	}

	// Get artist
	if artist, ok := mediaData["artist"].(string); ok && artist != "" {
		mediaInfo.Artist = artist
	}

	// Get album
	if album, ok := mediaData["album"].(string); ok && album != "" {
		mediaInfo.Album = album
	}

	// Get app name
	if appName, ok := mediaData["appName"].(string); ok && appName != "" {
		mediaInfo.AppName = appName
	}

	// Get duration (in seconds)
	duration := 0
	if d, ok := mediaData["duration"].(float64); ok {
		duration = int(d)
	} else if d, ok := mediaData["durationMicros"].(float64); ok {
		duration = int(d / 1000000)
	} else if d, ok := mediaData["totalTime"].(float64); ok {
		duration = int(d)
	} else if d, ok := mediaData["totalDuration"].(float64); ok {
		duration = int(d)
	}
	mediaInfo.Duration = duration

	// Get position (in seconds)
	position := 0
	if p, ok := mediaData["elapsedTime"].(float64); ok {
		position = int(p)
	} else if p, ok := mediaData["position"].(float64); ok {
		position = int(p)
	} else if p, ok := mediaData["positionMicros"].(float64); ok {
		position = int(p / 1000000)
	}
	mediaInfo.Position = position

	// Set state based on playing status
	mediaInfo.State = "playing"

	return mediaInfo, nil
}

// updateMediaPlayer updates the MQTT topics with current media player information
func (app *Application) updateMediaPlayer(client mqtt.Client) {
	mediaInfo, err := getMediaInfo()
	if err != nil {
		// Check if it's a Media Control error
		if _, ok := err.(*MediaControlError); ok {
			log.Printf("Media Control is not available: %v", err)
			log.Println("To install Media Control:")
			log.Println("  1. Install via npm: npm install -g media-control")
			log.Println("  2. Or install via Homebrew: brew install media-control")
			log.Println("Media player information will be disabled until Media Control is available")
		} else {
			log.Printf("Error getting media info: %v", err)
		}
		return
	}

	// If no media is playing, publish empty state
	if mediaInfo == nil {
		log.Println("No media playing - publishing idle state")
		app.publishMediaState(client, "idle", "", "", "", "", 0, 0)
		return
	}

	// Determine the state
	state := "idle"
	switch mediaInfo.State {
	case "playing":
		state = "playing"
	case "paused":
		state = "paused"
	case "stopped":
		state = "idle"
	}

	log.Printf("Media playing: %s - %s (%s)", mediaInfo.Artist, mediaInfo.Title, state)
	app.publishMediaState(client, state, mediaInfo.Title, mediaInfo.Artist, mediaInfo.Album, mediaInfo.AppName, mediaInfo.Duration, mediaInfo.Position)
}

// updateNowPlaying updates the now playing sensor with current media information
func (app *Application) updateNowPlaying(client mqtt.Client) {
	mediaInfo, err := getMediaInfo()
	if err != nil {
		if _, ok := err.(*MediaControlError); ok {
			log.Printf("Media Control is not available: %v", err)
			return
			//revive:disable-next-line
		} else {
			log.Printf("Error getting media info: %v", err)
			return
		}
	}

	// If no media is playing, publish idle state
	if mediaInfo == nil {
		state := "idle"
		client.Publish(app.getTopicPrefix()+"/status/now_playing", 0, false, state)
		attr := map[string]interface{}{
			"state":    state,
			"title":    "",
			"artist":   "",
			"album":    "",
			"app_name": "",
			"duration": 0,
			"position": 0,
		}
		attrJSON, _ := json.Marshal(attr)
		client.Publish(app.getTopicPrefix()+"/status/now_playing_attr", 0, false, string(attrJSON))
		return
	}

	// Determine the state
	state := "idle"
	switch mediaInfo.State {
	case "playing":
		state = "playing"
	case "paused":
		state = "paused"
	case "stopped":
		state = "idle"
	}

	// Publish state and attributes
	client.Publish(app.getTopicPrefix()+"/status/now_playing", 0, false, state)
	attr := map[string]interface{}{
		"state":    state,
		"title":    mediaInfo.Title,
		"artist":   mediaInfo.Artist,
		"album":    mediaInfo.Album,
		"app_name": mediaInfo.AppName,
		"duration": mediaInfo.Duration,
		"position": mediaInfo.Position,
	}
	attrJSON, _ := json.Marshal(attr)
	client.Publish(app.getTopicPrefix()+"/status/now_playing_attr", 0, false, string(attrJSON))
	log.Printf("Updated now playing sensor: %s - %s (%s)", mediaInfo.Artist, mediaInfo.Title, state)
}

// startMediaStream starts the media-control stream for real-time updates
func (app *Application) startMediaStream(client mqtt.Client) {
	if !isMediaControlAvailable() {
		log.Println("Media Control not available - skipping media stream")
		return
	}

	log.Println("Starting media-control stream for real-time updates...")

	cmd := exec.Command("media-control", "stream")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error creating stdout pipe for media stream: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error starting media-control stream: %v", err)
		return
	}

	// Read the stream in a goroutine with error recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Media stream goroutine recovered from panic: %v", r)
			}
			cmd.Wait()
			stdout.Close()
		}()

		scanner := bufio.NewScanner(stdout)
		// Increase buffer size to handle long JSON lines from media-control stream
		buf := make([]byte, 0, 64*1024) // 64KB buffer
		scanner.Buffer(buf, 1024*1024)  // Allow up to 1MB tokens

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Parse the JSON line from the stream
			var mediaData map[string]interface{}
			if err := json.Unmarshal([]byte(line), &mediaData); err != nil {
				log.Printf("Error parsing media stream JSON: %v", err)
				continue
			}

			// Process the media update only if MQTT client is connected
			if client.IsConnected() {
				app.processMediaStreamUpdate(client, mediaData)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Error reading media stream: %v", err)
			log.Println("Media stream will restart on next application restart")
		}
	}()

	log.Println("Media stream started successfully")
}

// processMediaStreamUpdate processes a single media update from the stream
func (app *Application) processMediaStreamUpdate(client mqtt.Client, mediaData map[string]interface{}) {
	// The stream sends {"type":"data","diff":true,"payload":{...}}
	// Only update fields present in payload
	payload, ok := mediaData["payload"].(map[string]interface{})
	if !ok {
		log.Printf("Media stream: No payload in event, skipping")
		return
	}

	// Merge payload into currentMediaState
	for k, v := range payload {
		switch k {
		case "title":
			if s, ok := v.(string); ok {
				app.currentMediaState.Title = s
			}
		case "artist":
			if s, ok := v.(string); ok {
				app.currentMediaState.Artist = s
			}
		case "album":
			if s, ok := v.(string); ok {
				app.currentMediaState.Album = s
			}
		case "appName":
			if s, ok := v.(string); ok {
				app.currentMediaState.AppName = s
			}
		case "bundleIdentifier":
			if s, ok := v.(string); ok {
				app.currentMediaState.AppName = s
			}
		case "playing":
			if b, ok := v.(bool); ok {
				if b {
					app.currentMediaState.State = "playing"
				} else {
					app.currentMediaState.State = "paused"
				}
			}
		case "duration":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Duration = int(f)
			}
		case "durationMicros":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Duration = int(f / 1000000)
			}
		case "totalTime":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Duration = int(f)
			}
		case "totalDuration":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Duration = int(f)
			}
		case "elapsedTime":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Position = int(f)
			}
		case "position":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Position = int(f)
			}
		case "positionMicros":
			if f, ok := v.(float64); ok {
				app.currentMediaState.Position = int(f / 1000000)
			}
		}
	}

	// If playing is false and no other info, treat as idle
	if state, ok := payload["playing"]; ok {
		if b, ok := state.(bool); ok && !b {
			app.currentMediaState.State = "idle"
		}
	}

	// Publish state and attributes
	client.Publish(app.getTopicPrefix()+"/status/now_playing", 0, false, app.currentMediaState.State)
	attr := map[string]interface{}{
		"state":    app.currentMediaState.State,
		"title":    app.currentMediaState.Title,
		"artist":   app.currentMediaState.Artist,
		"album":    app.currentMediaState.Album,
		"app_name": app.currentMediaState.AppName,
		"duration": app.currentMediaState.Duration,
		"position": app.currentMediaState.Position,
	}
	attrJSON, _ := json.Marshal(attr)
	client.Publish(app.getTopicPrefix()+"/status/now_playing_attr", 0, false, string(attrJSON))
	log.Printf("Media stream update: %s - %s (%s)", app.currentMediaState.Artist, app.currentMediaState.Title, app.currentMediaState.State)
}

// getUserActivityState gets the current user activity state
func (app *Application) getUserActivityState() string {
	app.activityMutex.RLock()
	defer app.activityMutex.RUnlock()
	return app.userActivityState
}

// setUserActivityState sets the user activity state and publishes to MQTT
func (app *Application) setUserActivityState(client mqtt.Client, state string) {
	app.activityMutex.Lock()
	defer app.activityMutex.Unlock()

	if app.userActivityState != state {
		app.userActivityState = state
		if client != nil && client.IsConnected() {
			client.Publish(app.getTopicPrefix()+"/status/user_activity", 0, false, state)
			log.Printf("User activity state changed to: %s", state)
		}
	}
}

// resetActivityTimer resets the inactivity timer
func (app *Application) resetActivityTimer(client mqtt.Client) {
	app.activityMutex.Lock()
	defer app.activityMutex.Unlock()

	// Set to active immediately
	if app.userActivityState != "active" {
		app.userActivityState = "active"
		if client != nil && client.IsConnected() {
			client.Publish(app.getTopicPrefix()+"/status/user_activity", 0, false, "active")
			log.Printf("User activity detected - state: active")
		}
	}

	// Reset or create the timer
	if app.activityTimer != nil {
		app.activityTimer.Stop()
	}

	app.activityTimer = time.AfterFunc(time.Duration(app.config.IdleActivityTime)*time.Second, func() {
		app.setUserActivityState(client, "inactive")
	})
}

// getSystemIdleTime gets the system idle time in seconds
func getSystemIdleTime() (int, error) {
	cmd := exec.Command("ioreg", "-c", "IOHIDSystem")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("error running ioreg: %w", err)
	}

	// Parse the HIDIdleTime from the output
	re := regexp.MustCompile(`"HIDIdleTime" = (\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return 0, fmt.Errorf("HIDIdleTime not found in ioreg output")
	}

	idleTimeNanos, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing idle time: %w", err)
	}

	// Convert nanoseconds to seconds
	idleTimeSeconds := int(idleTimeNanos / 1000000000)
	return idleTimeSeconds, nil
}

// startUserActivityMonitoring starts monitoring user activity using system idle time
func (app *Application) startUserActivityMonitoring(client mqtt.Client) {
	log.Println("Starting user activity monitoring...")

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Activity monitor goroutine recovered from panic: %v", r)
			}
		}()

		var lastIdleTime int = -1

		for {
			// Check if client is still connected
			if client == nil || !client.IsConnected() {
				time.Sleep(5 * time.Second)
				continue
			}

			idleTime, err := getSystemIdleTime()
			if err != nil {
				log.Printf("Error getting system idle time: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			// If idle time decreased or is very small, user is active
			if idleTime < lastIdleTime || idleTime < 2 {
				app.resetActivityTimer(client)
			}

			lastIdleTime = idleTime
			client.Publish(app.getTopicPrefix()+"/status/idle_time_seconds", 0, false, fmt.Sprintf("%d", idleTime))
			// Check every 500ms for responsive detection
			time.Sleep(500 * time.Millisecond)
		}
	}()

	log.Println("User activity monitoring started successfully")
}

// publishMediaState publishes the current media state to MQTT
func (app *Application) publishMediaState(client mqtt.Client, state, title, artist, album, appName string, duration, position int) {
	// Publish individual attributes
	client.Publish(app.getTopicPrefix()+"/status/media_state", 0, false, state)
	client.Publish(app.getTopicPrefix()+"/status/media_title", 0, false, title)
	client.Publish(app.getTopicPrefix()+"/status/media_artist", 0, false, artist)
	client.Publish(app.getTopicPrefix()+"/status/media_album", 0, false, album)
	client.Publish(app.getTopicPrefix()+"/status/media_app", 0, false, appName)
	client.Publish(app.getTopicPrefix()+"/status/media_duration", 0, false, strconv.Itoa(duration))
	client.Publish(app.getTopicPrefix()+"/status/media_position", 0, false, strconv.Itoa(position))

	// Publish combined JSON state for media_player entity
	mediaState := map[string]interface{}{
		"state":        state,
		"title":        title,
		"artist":       artist,
		"album":        album,
		"app_name":     appName,
		"duration":     duration,
		"position":     position,
		"media_title":  title,
		"media_artist": artist,
		"media_album":  album,
	}

	stateJSON, _ := json.Marshal(mediaState)
	mediaPlayerTopic := app.getTopicPrefix() + "/status/media_player"
	client.Publish(mediaPlayerTopic, 0, false, string(stateJSON))
	log.Printf("Published media state to %s: %s", mediaPlayerTopic, string(stateJSON))
}

// updateDisplayBrightness updates the MQTT topics with current display brightness values
func (app *Application) updateDisplayBrightness(client mqtt.Client) {
	// Skip if no displays are available
	if len(app.displays) == 0 {
		return
	}

	// Refresh display list to handle dynamic display changes (laptop open/close)
	currentDisplays := getDisplays()
	if currentDisplays != nil {
		app.displays = currentDisplays
	}

	for _, display := range app.displays {
		brightness, err := getDisplayBrightness(display.DisplayID)
		if err != nil {
			// Only log error once per minute to avoid spam for unavailable displays (e.g., closed laptop)
			if display.Name == "Built-in Display" || strings.Contains(display.Name, "Built-in") {
				// Silently skip built-in display when unavailable (laptop closed)
				continue
			}
			log.Printf("Error getting brightness for display %s: %v", display.Name, err)
			// Check if it's a BetterDisplay CLI error
			if !isBetterDisplayCLIAvailable() {
				log.Printf("BetterDisplay CLI is not available for display %s", display.Name)
			}
			continue
		}

		statusTopic := app.getTopicPrefix() + "/status/display_" + display.DisplayID + "_brightness"
		client.Publish(statusTopic, 0, true, strconv.Itoa(brightness))
	}
}

func (app *Application) messagePubHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
	app.listen(client, msg)
}

func (app *Application) connectHandler(client mqtt.Client) {
	log.Println("Connected to MQTT")

	// Set up device configuration (in case this is a reconnection)
	app.setDevice(client)

	token := client.Publish(app.getTopicPrefix()+"/status/alive", 0, true, "online")
	token.Wait()

	log.Println("Sending 'online' to topic: " + app.getTopicPrefix() + "/status/alive")
	app.sub(client, app.getTopicPrefix()+"/command/#")

	// Start media stream if not already running (for reconnections)
	if isMediaControlAvailable() {
		go app.startMediaStream(client)
	}

	// Start user activity monitoring
	go app.startUserActivityMonitoring(client)

	// Send initial state updates
	app.updateVolume(client)
	app.updateMute(client)
	app.updateCaffeinateStatus(client)
	app.updateDisplayBrightness(client)
	app.updateNowPlaying(client)
	app.setUserActivityState(client, "inactive") // Initial state
}

func (app *Application) connectLostHandler(_ mqtt.Client, err error) {
	log.Printf("Disconnected from MQTT: %v", err)

	// Check if it's a network issue
	if !app.isNetworkReachable() {
		log.Println("MQTT broker is not reachable - likely on a different network")
		log.Println("Will retry connection when network becomes available")
	} else {
		log.Println("MQTT client will attempt to reconnect automatically...")
	}
}

func (app *Application) getMQTTClient() error {
	return app.getMQTTClientWithRetry(0)
}

// isNetworkReachable checks if the MQTT broker is reachable before attempting connection
func (app *Application) isNetworkReachable() bool {
	// Try to connect to the broker with a short timeout
	timeout := 5 * time.Second
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(app.config.IP, app.config.Port), timeout)
	if err != nil {
		log.Printf("Network check failed: MQTT broker %s:%s is not reachable (%v)", app.config.IP, app.config.Port, err)
		return false
	}
	conn.Close()
	return true
}

func (app *Application) getMQTTClientWithRetry(retryCount int) error {
	// Prevent infinite recursion
	if retryCount > MaxRetryAttempts {
		return fmt.Errorf("failed to connect to MQTT broker after multiple attempts")
	}

	// Check network reachability first to avoid long timeouts
	if !app.isNetworkReachable() {
		log.Printf("MQTT broker is not reachable on current network, will retry later")
		return fmt.Errorf("MQTT broker not reachable")
	}

	opts := mqtt.NewClientOptions()

	// Determine protocol and broker URL
	protocol := "tcp"
	if app.config.SSL {
		protocol = "ssl"
	}
	brokerURL := fmt.Sprintf("%s://%s:%s", protocol, app.config.IP, app.config.Port)
	log.Printf("Connecting to MQTT broker: %s", brokerURL)

	opts.AddBroker(brokerURL)
	if app.config.User != "" {
		opts.SetUsername(app.config.User)
	}
	if app.config.Password != "" {
		opts.SetPassword(app.config.Password)
	}

	// Set up handlers with application context
	opts.OnConnect = app.connectHandler
	opts.OnConnectionLost = app.connectLostHandler
	opts.SetDefaultPublishHandler(app.messagePubHandler)

	// Set client ID to ensure unique identification with timestamp to avoid conflicts
	clientID := fmt.Sprintf("%s_mac2mqtt_%d", app.hostname, time.Now().Unix())
	opts.SetClientID(clientID)

	// Network-aware connection reliability settings
	opts.SetClientID(app.hostname + "_mac2mqtt")
	opts.SetKeepAlive(60 * time.Second)      // Send ping every 60 seconds
	opts.SetPingTimeout(10 * time.Second)    // Shorter ping timeout for faster network change detection
	opts.SetConnectTimeout(15 * time.Second) // Shorter connect timeout for network switching
	opts.SetAutoReconnect(true)              // Enable auto-reconnect
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(15 * time.Second) // Wait 15 seconds between retries (good for network switches)
	opts.SetMaxReconnectInterval(2 * time.Minute)  // Max 2 minutes between reconnect attempts (faster recovery)
	opts.SetCleanSession(false)                    // Resume session to avoid losing subscriptions
	opts.SetOrderMatters(false)                    // Allow out-of-order delivery for better performance
	opts.SetWriteTimeout(10 * time.Second)         // Shorter write timeout for network issues
	opts.SetResumeSubs(true)                       // Resume subscriptions on reconnect

	// Set will message
	opts.SetWill(app.getTopicPrefix()+"/status/alive", "offline", 0, true)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		// If SSL connection fails, try falling back to non-SSL
		if app.config.SSL {
			log.Printf("SSL connection failed: %v. Trying non-SSL connection...", token.Error())
			app.config.SSL = false
			return app.getMQTTClientWithRetry(retryCount + 1)
		}
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	app.client = client
	return nil
}

func (app *Application) sub(client mqtt.Client, topic string) {
	token := client.Subscribe(topic, 0, nil)
	token.Wait()
	log.Printf("Subscribed to topic: %s\n", topic)
}

func (app *Application) listen(client mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := string(msg.Payload())

	// Handle volume commands
	if app.handleVolumeCommand(client, topic, payload) {
		return
	}

	// Handle mute commands
	if app.handleMuteCommand(client, topic, payload) {
		return
	}

	// Handle system commands
	if app.handleSystemCommand(topic, payload) {
		return
	}

	// Handle display brightness commands
	if app.handleDisplayBrightnessCommand(client, topic, payload) {
		return
	}

	// Handle shortcut commands
	if app.handleShortcutCommand(topic, payload) {
		return
	}

	// Handle keep awake commands
	if app.handleKeepAwakeCommand(client, topic, payload) {
		return
	}

	// Handle play/pause commands
	if app.handlePlayPauseCommand(client, topic, payload) {
		return
	}
}

// handleVolumeCommand handles volume control commands
func (app *Application) handleVolumeCommand(client mqtt.Client, topic, payload string) bool {
	if topic != app.getTopicPrefix()+"/command/volume" {
		return false
	}

	volume, err := app.validateVolumeInput(payload)
	if err != nil {
		log.Printf("Invalid volume value: %v", err)
		return true
	}

	setVolume(volume)
	app.updateVolume(client)
	app.updateMute(client)
	return true
}

// handleMuteCommand handles mute control commands
func (app *Application) handleMuteCommand(client mqtt.Client, topic, payload string) bool {
	if topic != app.getTopicPrefix()+"/command/mute" {
		return false
	}

	mute, err := app.validateMuteInput(payload)
	if err != nil {
		log.Printf("Invalid mute value: %v", err)
		return true
	}

	setMute(mute)
	app.updateVolume(client)
	app.updateMute(client)
	return true
}

// handleSystemCommand handles system control commands
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

// handleDisplayBrightnessCommand handles display brightness commands
func (app *Application) handleDisplayBrightnessCommand(client mqtt.Client, topic, payload string) bool {
	// Check if we have any displays available
	if len(app.displays) == 0 {
		log.Printf("Received display brightness command but no displays are available")
		log.Printf("Topic: %s, Payload: %s", topic, payload)
		log.Println("This usually means BetterDisplay CLI is not installed or not accessible")
		return true // Return true to indicate we handled the command
	}

	for _, display := range app.displays {
		commandTopic := app.getTopicPrefix() + "/command/display_" + display.DisplayID + "_brightness"
		if topic == commandTopic {
			brightness, err := app.validateBrightnessInput(payload)
			if err != nil {
				log.Printf("Invalid brightness value for display %s: %v", display.Name, err)
				return true
			}

			err = setDisplayBrightness(display.DisplayID, brightness)
			if err != nil {
				log.Printf("Error setting brightness for display %s: %v", display.Name, err)
				// Check if it's a BetterDisplay CLI error
				if !isBetterDisplayCLIAvailable() {
					log.Println("BetterDisplay CLI is not available. Please install BetterDisplay and enable CLI access.")
				}
			} else {
				// Update the status immediately
				statusTopic := app.getTopicPrefix() + "/status/display_" + display.DisplayID + "_brightness"
				client.Publish(statusTopic, 0, true, strconv.Itoa(brightness))
			}
			return true
		}
	}
	return false
}

// handleShortcutCommand handles shortcut execution commands
func (app *Application) handleShortcutCommand(topic, payload string) bool {
	if topic != app.getTopicPrefix()+"/command/runshortcut" {
		return false
	}

	if err := app.validateShortcutInput(payload); err != nil {
		log.Printf("Invalid shortcut: %v", err)
		return true
	}

	commandRunShortcut(payload)
	return true
}

// handleKeepAwakeCommand handles keep awake commands
func (app *Application) handleKeepAwakeCommand(client mqtt.Client, topic, payload string) bool {
	if topic != app.getTopicPrefix()+"/command/keepawake" {
		return false
	}

	keepAwake, err := app.validateKeepAwakeInput(payload)
	if err != nil {
		log.Printf("Invalid keep awake value: %v", err)
		return true
	}

	if keepAwake {
		commandKeepAwake()
	} else {
		commandAllowSleep()
	}
	app.updateCaffeinateStatus(client)
	return true
}

// handlePlayPauseCommand handles play/pause commands
func (app *Application) handlePlayPauseCommand(client mqtt.Client, topic, payload string) bool {
	if topic != app.getTopicPrefix()+"/command/playpause" {
		return false
	}

	if payload == "playpause" {
		commandPlayPause()
		// Update the now playing sensor after a short delay to reflect the new state
		time.Sleep(500 * time.Millisecond)
		app.updateNowPlaying(client)
	}
	return true
}

func (app *Application) updateVolume(client mqtt.Client) {
	token := client.Publish(app.getTopicPrefix()+"/status/volume", 0, false, strconv.Itoa(getCurrentVolume()))
	token.Wait()
}

func (app *Application) updateMute(client mqtt.Client) {
	token := client.Publish(app.getTopicPrefix()+"/status/mute", 0, false, strconv.FormatBool(getMuteStatus()))
	token.Wait()
}

func getBatteryChargePercent() string {

	output := getCommandOutput("/usr/bin/pmset", "-g", "batt")

	// $ /usr/bin/pmset -g batt
	// Now drawing from 'Battery Power'
	//  -InternalBattery-0 (id=4653155)        100%; discharging; 20:00 remaining present: true

	r := regexp.MustCompile(`(\d+)%`)
	res := r.FindStringSubmatch(output)
	if len(res) == 0 {
		return ""
	}

	return res[1]
}

// DiskUsage holds disk usage statistics
type DiskUsage struct {
	Total       uint64  `json:"total"`        // Total bytes
	Used        uint64  `json:"used"`         // Used bytes
	Free        uint64  `json:"free"`         // Free bytes
	UsedPercent float64 `json:"used_percent"` // Used percentage
	FreePercent float64 `json:"free_percent"` // Free percentage
}

func getDiskUsage() (*DiskUsage, error) {
	fs := sigar.FileSystemList{}
	if err := fs.Get(); err != nil {
		return nil, fmt.Errorf("failed to get filesystem list: %w", err)
	}

	// Find the root filesystem
	for _, filesystem := range fs.List {
		if filesystem.DirName == "/" {
			usage := sigar.FileSystemUsage{}
			if err := usage.Get(filesystem.DirName); err != nil {
				return nil, fmt.Errorf("failed to get disk usage: %w", err)
			}

			// Convert from KB to bytes (gosigar returns values in KB)
			totalBytes := usage.Total * 1024
			usedBytes := usage.Used * 1024
			freeBytes := usage.Free * 1024

			// Calculate percentages
			usedPercent := float64(0)
			freePercent := float64(0)
			if totalBytes > 0 {
				usedPercent = float64(usedBytes) / float64(totalBytes) * 100
				freePercent = float64(freeBytes) / float64(totalBytes) * 100
			}

			return &DiskUsage{
				Total:       totalBytes,
				Used:        usedBytes,
				Free:        freeBytes,
				UsedPercent: usedPercent,
				FreePercent: freePercent,
			}, nil
		}
	}

	return nil, fmt.Errorf("root filesystem not found")
}

// CPUUsage holds CPU usage statistics
type CPUUsage struct {
	UsedPercent float64 `json:"used_percent"` // CPU used percentage
	FreePercent float64 `json:"free_percent"` // CPU idle/free percentage
}

// MemoryUsage holds memory usage statistics
type MemoryUsage struct {
	Total       uint64  `json:"total"`        // Total bytes
	Used        uint64  `json:"used"`         // Used bytes
	Free        uint64  `json:"free"`         // Free bytes
	UsedPercent float64 `json:"used_percent"` // Used percentage
	FreePercent float64 `json:"free_percent"` // Free percentage
}

// UptimeInfo holds system uptime information
type UptimeInfo struct {
	Seconds uint64 `json:"seconds"` // Uptime in seconds
	Human   string `json:"human"`   // Human-readable format
}

func (app *Application) getCPUUsage() (*CPUUsage, error) {
	cpu := sigar.Cpu{}
	if err := cpu.Get(); err != nil {
		return nil, fmt.Errorf("failed to get CPU stats: %w", err)
	}

	app.cpuMutex.Lock()
	defer app.cpuMutex.Unlock()

	// Calculate the delta since last measurement
	userDelta := cpu.User - app.lastCPU.User
	sysDelta := cpu.Sys - app.lastCPU.Sys
	idleDelta := cpu.Idle - app.lastCPU.Idle
	waitDelta := cpu.Wait - app.lastCPU.Wait
	niceDelta := cpu.Nice - app.lastCPU.Nice
	irqDelta := cpu.Irq - app.lastCPU.Irq
	softIrqDelta := cpu.SoftIrq - app.lastCPU.SoftIrq
	stolenDelta := cpu.Stolen - app.lastCPU.Stolen

	// Calculate total time delta
	totalDelta := userDelta + sysDelta + idleDelta + waitDelta + niceDelta + irqDelta + softIrqDelta + stolenDelta

	// Store current CPU stats for next calculation
	app.lastCPU = cpu

	// If this is the first measurement or total is zero, return 0% usage
	if totalDelta == 0 {
		return &CPUUsage{
			UsedPercent: 0,
			FreePercent: 100,
		}, nil
	}

	// Calculate idle and used percentages
	idlePercent := float64(idleDelta) / float64(totalDelta) * 100
	usedPercent := 100 - idlePercent

	return &CPUUsage{
		UsedPercent: usedPercent,
		FreePercent: idlePercent,
	}, nil
}

func getMemoryUsage() (*MemoryUsage, error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		log.Fatalf("Error getting virtual memory stats: %v", err)
	}

	total := vmStat.Total
	used := vmStat.Used
	free := vmStat.Free

	return &MemoryUsage{
		Total:       total,
		Used:        used,
		Free:        free,
		UsedPercent: vmStat.UsedPercent,
		FreePercent: 100 - vmStat.UsedPercent,
	}, nil
}

func getSystemUptime() (*UptimeInfo, error) {
	uptime := sigar.Uptime{}
	if err := uptime.Get(); err != nil {
		return nil, fmt.Errorf("failed to get uptime: %w", err)
	}

	// Convert to human-readable format
	totalSeconds := uint64(uptime.Length)
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60

	var uptimeHuman string
	if days > 0 {
		uptimeHuman = fmt.Sprintf("%d days, %d:%02d", days, hours, minutes)
	} else {
		uptimeHuman = fmt.Sprintf("%d:%02d", hours, minutes)
	}

	return &UptimeInfo{
		Seconds: totalSeconds,
		Human:   uptimeHuman,
	}, nil
}

func (app *Application) updateBattery(client mqtt.Client) {
	token := client.Publish(app.getTopicPrefix()+"/status/battery", 0, false, getBatteryChargePercent())
	token.Wait()
}

func (app *Application) updateCaffeinateStatus(client mqtt.Client) {
	token := client.Publish(app.getTopicPrefix()+"/status/caffeinate", 0, false, strconv.FormatBool(getCaffeinateStatus()))
	token.Wait()
}

func (app *Application) updateDiskUsage(client mqtt.Client) {
	diskUsage, err := getDiskUsage()
	if err != nil {
		log.Printf("Failed to get disk usage: %v", err)
		return
	}

	// Publish individual metrics
	client.Publish(app.getTopicPrefix()+"/status/disk/total", 0, false, fmt.Sprintf("%d", diskUsage.Total))
	client.Publish(app.getTopicPrefix()+"/status/disk/used", 0, false, fmt.Sprintf("%d", diskUsage.Used))
	client.Publish(app.getTopicPrefix()+"/status/disk/free", 0, false, fmt.Sprintf("%d", diskUsage.Free))
	client.Publish(app.getTopicPrefix()+"/status/disk/used_percent", 0, false, fmt.Sprintf("%.2f", diskUsage.UsedPercent))
	client.Publish(app.getTopicPrefix()+"/status/disk/free_percent", 0, false, fmt.Sprintf("%.2f", diskUsage.FreePercent))
}

func (app *Application) updateCPUUsage(client mqtt.Client) {
	cpuUsage, err := app.getCPUUsage()
	if err != nil {
		log.Printf("Failed to get CPU usage: %v", err)
		return
	}

	// Publish CPU metrics
	client.Publish(app.getTopicPrefix()+"/status/cpu/used_percent", 0, false, fmt.Sprintf("%.2f", cpuUsage.UsedPercent))
	client.Publish(app.getTopicPrefix()+"/status/cpu/free_percent", 0, false, fmt.Sprintf("%.2f", cpuUsage.FreePercent))
}

func (app *Application) updateMemoryUsage(client mqtt.Client) {
	memUsage, err := getMemoryUsage()
	if err != nil {
		log.Printf("Failed to get memory usage: %v", err)
		return
	}

	// Publish memory metrics
	client.Publish(app.getTopicPrefix()+"/status/memory/total", 0, false, fmt.Sprintf("%d", memUsage.Total))
	client.Publish(app.getTopicPrefix()+"/status/memory/used", 0, false, fmt.Sprintf("%d", memUsage.Used))
	client.Publish(app.getTopicPrefix()+"/status/memory/free", 0, false, fmt.Sprintf("%d", memUsage.Free))
	client.Publish(app.getTopicPrefix()+"/status/memory/used_percent", 0, false, fmt.Sprintf("%.2f", memUsage.UsedPercent))
	client.Publish(app.getTopicPrefix()+"/status/memory/free_percent", 0, false, fmt.Sprintf("%.2f", memUsage.FreePercent))
}

func (app *Application) updateUptime(client mqtt.Client) {
	uptime, err := getSystemUptime()
	if err != nil {
		log.Printf("Failed to get uptime: %v", err)
		return
	}

	// Publish uptime metrics
	client.Publish(app.getTopicPrefix()+"/status/uptime/seconds", 0, false, fmt.Sprintf("%d", uptime.Seconds))
	client.Publish(app.getTopicPrefix()+"/status/uptime/human", 0, false, uptime.Human)
}

func getMediaDevicesState() (bool, bool, error) {
	isMicOn, err := mediadevices.IsMicrophoneOn()
	if err != nil {
		return false, false, fmt.Errorf("failed to get microphone state: %w", err)
	}

	isCameraOn, err := mediadevices.IsCameraOn()
	if err != nil {
		return isMicOn, false, fmt.Errorf("failed to get camera state: %w", err)
	}

	return isMicOn, isCameraOn, nil
}

func (app *Application) updateMediaDevices(client mqtt.Client) {
	isMicOn, isCameraOn, err := getMediaDevicesState()
	if err != nil {
		log.Printf("Failed to get media devices state: %v", err)
		// Publish "unknown" state on error
		client.Publish(app.getTopicPrefix()+"/status/microphone", 0, false, "OFF")
		client.Publish(app.getTopicPrefix()+"/status/camera", 0, false, "OFF")
		return
	}

	// Publish media device states
	micState := "OFF"
	if isMicOn {
		micState = "ON"
	}
	cameraState := "OFF"
	if isCameraOn {
		cameraState = "ON"
	}

	client.Publish(app.getTopicPrefix()+"/status/microphone", 0, false, micState)
	client.Publish(app.getTopicPrefix()+"/status/camera", 0, false, cameraState)
}

func getPublicIP() (string, error) {
	// Use DNS over HTTPS to query Cloudflare's whoami service
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, "udp", "ns1.google.com:53")

		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query TXT record from Cloudflare's whoami service
	txtRecords, err := resolver.LookupTXT(ctx, "o-o.myaddr.l.google.com")
	if err != nil {
		return "", fmt.Errorf("failed to lookup public IP via DNS: %w", err)
	}

	if len(txtRecords) > 0 {
		return txtRecords[0], nil
	}

	return "", fmt.Errorf("no IP address found in DNS response")
}

func (app *Application) updatePublicIP(client mqtt.Client) {
	publicIP, err := getPublicIP()
	if err != nil {
		log.Printf("Failed to get public IP: %v", err)
		// Publish empty string on error
		client.Publish(app.getTopicPrefix()+"/status/public_ip", 0, false, "unavailable")
		return
	}

	// Publish public IP
	client.Publish(app.getTopicPrefix()+"/status/public_ip", 0, false, publicIP)
}

func (app *Application) setDevice(client mqtt.Client) {

	keepawake := map[string]interface{}{
		"p":             "switch",
		"name":          "Keep Awake",
		"unique_id":     app.hostname + "_keepwake",
		"command_topic": app.getTopicPrefix() + "/command/keepawake",
		"payload_on":    "true",
		"payload_off":   "false",
		"state_topic":   app.getTopicPrefix() + "/status/caffeinate",
		"icon":          "mdi:coffee",
	}

	displaywake := map[string]interface{}{
		"p":             "button",
		"name":          "Display Wake",
		"unique_id":     app.hostname + "_displaywake",
		"command_topic": app.getTopicPrefix() + "/command/set",
		"payload_press": "displaywake",
		"icon":          "mdi:monitor",
	}

	displaysleep := map[string]interface{}{
		"p":             "button",
		"name":          "Display Sleep",
		"unique_id":     app.hostname + "_displaysleep",
		"command_topic": app.getTopicPrefix() + "/command/set",
		"payload_press": "displaysleep",
		"icon":          "mdi:monitor-off",
	}

	screensaver := map[string]interface{}{
		"p":             "button",
		"name":          "Screensaver",
		"unique_id":     app.hostname + "_screensaver",
		"command_topic": app.getTopicPrefix() + "/command/set",
		"payload_press": "screensaver",
		"icon":          "mdi:monitor-star",
	}

	sleep := map[string]interface{}{
		"p":             "button",
		"name":          "Sleep",
		"unique_id":     app.hostname + "_sleep",
		"command_topic": app.getTopicPrefix() + "/command/set",
		"payload_press": "sleep",
		"icon":          "mdi:sleep",
	}

	shutdown := map[string]interface{}{
		"p":                  "button",
		"name":               "Shutdown",
		"unique_id":          app.hostname + "_shutdown",
		"command_topic":      app.getTopicPrefix() + "/command/set",
		"payload_press":      "shutdown",
		"enabled_by_default": false,
		"icon":               "mdi:power",
	}
	mute := map[string]interface{}{
		"p":             "switch",
		"name":          "Mute",
		"unique_id":     app.hostname + "_mute",
		"command_topic": app.getTopicPrefix() + "/command/mute",
		"payload_on":    "true",
		"payload_off":   "false",
		"state_topic":   app.getTopicPrefix() + "/status/mute",
		"icon":          "mdi:volume-mute",
	}

	volume := map[string]interface{}{
		"p":             "number",
		"name":          "Volume",
		"unique_id":     app.hostname + "_volume",
		"command_topic": app.getTopicPrefix() + "/command/volume",
		"state_topic":   app.getTopicPrefix() + "/status/volume",
		"min_value":     MinVolume,
		"max_value":     MaxVolume,
		"step":          1,
		"mode":          "slider",
		"icon":          "mdi:volume-high",
	}

	battery := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Battery",
		"unique_id":           app.hostname + "_battery",
		"state_topic":         app.getTopicPrefix() + "/status/battery",
		"enabled_by_default":  false,
		"unit_of_measurement": "%",
		"device_class":        "battery",
	}

	diskTotal := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Disk Total",
		"unique_id":           app.hostname + "_disk_total",
		"state_topic":         app.getTopicPrefix() + "/status/disk/total",
		"unit_of_measurement": "B",
		"device_class":        "data_size",
		"state_class":         "measurement",
		"icon":                "mdi:harddisk",
	}

	diskUsed := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Disk Used",
		"unique_id":           app.hostname + "_disk_used",
		"state_topic":         app.getTopicPrefix() + "/status/disk/used",
		"unit_of_measurement": "B",
		"device_class":        "data_size",
		"state_class":         "measurement",
		"icon":                "mdi:harddisk",
	}

	diskFree := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Disk Free",
		"unique_id":           app.hostname + "_disk_free",
		"state_topic":         app.getTopicPrefix() + "/status/disk/free",
		"unit_of_measurement": "B",
		"device_class":        "data_size",
		"state_class":         "measurement",
		"icon":                "mdi:harddisk",
	}

	diskUsedPercent := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Disk Used Percent",
		"unique_id":           app.hostname + "_disk_used_percent",
		"state_topic":         app.getTopicPrefix() + "/status/disk/used_percent",
		"unit_of_measurement": "%",
		"state_class":         "measurement",
		"icon":                "mdi:chart-pie",
	}

	diskFreePercent := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Disk Free Percent",
		"unique_id":           app.hostname + "_disk_free_percent",
		"state_topic":         app.getTopicPrefix() + "/status/disk/free_percent",
		"unit_of_measurement": "%",
		"state_class":         "measurement",
		"icon":                "mdi:chart-pie",
	}

	cpuUsedPercent := map[string]interface{}{
		"p":                   "sensor",
		"name":                "CPU Used Percent",
		"unique_id":           app.hostname + "_cpu_used_percent",
		"state_topic":         app.getTopicPrefix() + "/status/cpu/used_percent",
		"unit_of_measurement": "%",
		"state_class":         "measurement",
		"icon":                "mdi:cpu-64-bit",
	}

	cpuFreePercent := map[string]interface{}{
		"p":                   "sensor",
		"name":                "CPU Free Percent",
		"unique_id":           app.hostname + "_cpu_free_percent",
		"state_topic":         app.getTopicPrefix() + "/status/cpu/free_percent",
		"unit_of_measurement": "%",
		"state_class":         "measurement",
		"icon":                "mdi:cpu-64-bit",
	}

	memoryTotal := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Memory Total",
		"unique_id":           app.hostname + "_memory_total",
		"state_topic":         app.getTopicPrefix() + "/status/memory/total",
		"unit_of_measurement": "B",
		"device_class":        "data_size",
		"state_class":         "measurement",
		"icon":                "mdi:memory",
	}

	memoryUsed := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Memory Used",
		"unique_id":           app.hostname + "_memory_used",
		"state_topic":         app.getTopicPrefix() + "/status/memory/used",
		"unit_of_measurement": "B",
		"device_class":        "data_size",
		"state_class":         "measurement",
		"icon":                "mdi:memory",
	}

	memoryFree := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Memory Free",
		"unique_id":           app.hostname + "_memory_free",
		"state_topic":         app.getTopicPrefix() + "/status/memory/free",
		"unit_of_measurement": "B",
		"device_class":        "data_size",
		"state_class":         "measurement",
		"icon":                "mdi:memory",
	}

	memoryUsedPercent := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Memory Used Percent",
		"unique_id":           app.hostname + "_memory_used_percent",
		"state_topic":         app.getTopicPrefix() + "/status/memory/used_percent",
		"unit_of_measurement": "%",
		"state_class":         "measurement",
		"icon":                "mdi:memory",
	}

	memoryFreePercent := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Memory Free Percent",
		"unique_id":           app.hostname + "_memory_free_percent",
		"state_topic":         app.getTopicPrefix() + "/status/memory/free_percent",
		"unit_of_measurement": "%",
		"state_class":         "measurement",
		"icon":                "mdi:memory",
	}

	uptimeSeconds := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Uptime Seconds",
		"unique_id":           app.hostname + "_uptime_seconds",
		"state_topic":         app.getTopicPrefix() + "/status/uptime/seconds",
		"unit_of_measurement": "s",
		"device_class":        "duration",
		"state_class":         "total_increasing",
		"icon":                "mdi:clock-outline",
	}

	uptimeHuman := map[string]interface{}{
		"p":           "sensor",
		"name":        "Uptime",
		"unique_id":   app.hostname + "_uptime_human",
		"state_topic": app.getTopicPrefix() + "/status/uptime/human",
		"icon":        "mdi:clock-outline",
	}

	microphone := map[string]interface{}{
		"p":            "binary_sensor",
		"name":         "Microphone",
		"unique_id":    app.hostname + "_microphone",
		"state_topic":  app.getTopicPrefix() + "/status/microphone",
		"payload_on":   "ON",
		"payload_off":  "OFF",
		"icon":         "mdi:microphone",
		"device_class": "running",
	}

	camera := map[string]interface{}{
		"p":            "binary_sensor",
		"name":         "Camera",
		"unique_id":    app.hostname + "_camera",
		"state_topic":  app.getTopicPrefix() + "/status/camera",
		"payload_on":   "ON",
		"payload_off":  "OFF",
		"icon":         "mdi:camera",
		"device_class": "running",
	}

	publicIP := map[string]interface{}{
		"p":           "sensor",
		"name":        "Public IP",
		"unique_id":   app.hostname + "_public_ip",
		"state_topic": app.getTopicPrefix() + "/status/public_ip",
		"icon":        "mdi:ip-network",
	}

	components := map[string]interface{}{
		"sleep":               sleep,
		"shutdown":            shutdown,
		"volume":              volume,
		"mute":                mute,
		"displaywake":         displaywake,
		"displaysleep":        displaysleep,
		"screensaver":         screensaver,
		"battery":             battery,
		"keepawake":           keepawake,
		"disk_total":          diskTotal,
		"disk_used":           diskUsed,
		"disk_free":           diskFree,
		"disk_used_percent":   diskUsedPercent,
		"disk_free_percent":   diskFreePercent,
		"cpu_used_percent":    cpuUsedPercent,
		"cpu_free_percent":    cpuFreePercent,
		"memory_total":        memoryTotal,
		"memory_used":         memoryUsed,
		"memory_free":         memoryFree,
		"memory_used_percent": memoryUsedPercent,
		"memory_free_percent": memoryFreePercent,
		"uptime_seconds":      uptimeSeconds,
		"uptime_human":        uptimeHuman,
		"microphone":          microphone,
		"camera":              camera,
		"public_ip":           publicIP,
	}

	// Add user activity sensor
	userActivity := map[string]interface{}{
		"p":            "binary_sensor",
		"name":         "User Activity",
		"unique_id":    app.hostname + "_user_activity",
		"state_topic":  app.getTopicPrefix() + "/status/user_activity",
		"payload_on":   "active",
		"payload_off":  "inactive",
		"icon":         "mdi:account-check",
		"device_class": "occupancy",
	}
	components["user_activity"] = userActivity

	// Add idle time sensor
	idleTime := map[string]interface{}{
		"p":                   "sensor",
		"name":                app.hostname + " User Idle Time",
		"unique_id":           app.hostname + "_idle_time_seconds",
		"state_topic":         app.getTopicPrefix() + "/status/idle_time_seconds",
		"unit_of_measurement": "s",
		"device_class":        "duration",
		"state_class":         "measurement",
		"icon":                "mdi:timer-sand",
	}
	components["idle_time_seconds"] = idleTime

	// Add media control components if Media Control is available
	if isMediaControlAvailable() {
		playPause := map[string]interface{}{
			"p":             "button",
			"name":          "Play/Pause",
			"unique_id":     app.hostname + "_playpause",
			"command_topic": app.getTopicPrefix() + "/command/playpause",
			"payload_press": "playpause",
			"icon":          "mdi:play-pause",
		}

		nowPlaying := map[string]interface{}{
			"p":                     "sensor",
			"name":                  "Now Playing",
			"unique_id":             app.hostname + "_now_playing",
			"state_topic":           app.getTopicPrefix() + "/status/now_playing",
			"json_attributes_topic": app.getTopicPrefix() + "/status/now_playing_attr",
			"icon":                  "mdi:music",
		}

		components["playpause"] = playPause
		components["now_playing"] = nowPlaying
	}

	// Note: Media player will be published as separate standard MQTT autodiscovery message

	// Add display brightness controls for each display
	for _, display := range app.displays {
		displayBrightness := map[string]interface{}{
			"p":             "number",
			"name":          display.Name + " Brightness",
			"unique_id":     app.hostname + "_display_" + display.DisplayID + "_brightness",
			"command_topic": app.getTopicPrefix() + "/command/display_" + display.DisplayID + "_brightness",
			"state_topic":   app.getTopicPrefix() + "/status/display_" + display.DisplayID + "_brightness",
			"min_value":     MinBrightness,
			"max_value":     MaxBrightness,
			"step":          1,
			"mode":          "slider",
			"icon":          "mdi:brightness-6",
		}
		components["display_"+display.DisplayID+"_brightness"] = displayBrightness
	}

	origin := map[string]interface{}{
		"name": "mac2mqtt",
	}

	device := map[string]interface{}{
		"ids":  getSerialnumber(),
		"name": app.hostname,
		"mf":   "Apple",
		"mdl":  getModel(),
	}

	object := map[string]interface{}{
		"dev":                device,
		"o":                  origin,
		"cmps":               components,
		"availability_topic": app.getTopicPrefix() + "/status/alive",
		"qos":                2,
	}
	objectJSON, _ := json.Marshal(object)

	token := client.Publish(app.config.DiscoveryPrefix+"/device"+"/"+app.hostname+"/config", 0, true, objectJSON)
	token.Wait()

	// Note: Media player functionality replaced with play/pause button and now playing sensor
}

// handleOfflineMode manages application behavior when MQTT broker is unreachable
func (app *Application) handleOfflineMode() {
	log.Println("Operating in offline mode - MQTT broker not reachable")
	log.Println("Application will continue monitoring system state and attempt to reconnect periodically")

	// Continue basic system monitoring even when offline
	// This ensures the application doesn't crash and can recover when network returns
}

// Run starts the application and runs the main loop
func (app *Application) Run() error {
	log.Println("=== MAC2MQTT STARTING ===")
	log.Printf("Working directory: %s", getWorkingDirectory())
	log.Printf("Hostname set to: %s", app.hostname)
	log.Printf("Discovery Prefix: %s", app.config.DiscoveryPrefix)
	log.Printf("MQTT Broker: %s:%s", app.config.IP, app.config.Port)
	log.Printf("MQTT Topic: %s", app.topic)

	// Initialize displays before MQTT connection
	log.Println("=== DISCOVERING DISPLAYS ===")
	if len(app.displays) > 0 {
		log.Printf("Found %d display(s):", len(app.displays))
		for _, display := range app.displays {
			log.Printf("  - %s (ID: %s)", display.Name, display.DisplayID)
		}
	} else {
		log.Println("No displays found or BetterDisplay CLI not available")
	}
	log.Println("=== DISPLAY DISCOVERY COMPLETE ===")

	// Check Media Control availability
	log.Println("=== CHECKING MEDIA CONTROL ===")
	if isMediaControlAvailable() {
		log.Println("Media Control is available - Media player will be enabled")
	} else {
		log.Println("Media Control is not installed or not accessible")
		log.Println("To install Media Control:")
		log.Println("  1. Install via npm: npm install -g media-control")
		log.Println("  2. Or install via Homebrew: brew install media-control")
		log.Println("Media player information will be disabled until Media Control is available")
	}
	log.Println("=== MEDIA CONTROL CHECK COMPLETE ===")

	log.Println("Starting MQTT connection...")
	if err := app.getMQTTClient(); err != nil {
		log.Printf("Initial MQTT connection failed: %v", err)
		if !app.isNetworkReachable() {
			log.Println("MQTT broker not reachable - starting in offline mode")
			app.handleOfflineMode()
			// Continue running, the network check ticker will handle reconnection
		} else {
			return fmt.Errorf("failed to connect to MQTT: %w", err)
		}
	}

	// Set up tickers for periodic updates
	volumeTicker := time.NewTicker(UpdateInterval)
	batteryTicker := time.NewTicker(UpdateInterval)
	awakeTicker := time.NewTicker(UpdateInterval)
	networkCheckTicker := time.NewTicker(30 * time.Second) // Check network every 30 seconds
	defer volumeTicker.Stop()
	defer batteryTicker.Stop()
	defer awakeTicker.Stop()
	defer networkCheckTicker.Stop()

	// Track connection state
	lastConnectionState := app.isClientConnected()
	networkReachable := true

	// Initial setup - only if MQTT is connected
	if app.isClientConnected() {
		app.setDevice(app.client)
		app.updateVolume(app.client)
		app.updateMute(app.client)
		app.updateCaffeinateStatus(app.client)
		app.updateDisplayBrightness(app.client)
		app.updateNowPlaying(app.client)                 // Initial now playing update
		app.setUserActivityState(app.client, "inactive") // Initial user activity state
		app.updateDiskUsage(app.client)                  // Initial disk usage update
		app.updateCPUUsage(app.client)                   // Initial CPU usage update
		app.updateMemoryUsage(app.client)                // Initial memory usage update
		app.updateUptime(app.client)                     // Initial uptime update
		app.updateMediaDevices(app.client)               // Initial media devices update
		app.updatePublicIP(app.client)                   // Initial public IP update

		// Start media stream for real-time updates
		app.startMediaStream(app.client)

		// Start user activity monitoring
		app.startUserActivityMonitoring(app.client)
	} else {
		log.Println("Skipping initial MQTT setup - will configure when connection is established")
	}

	// Main event loop
	for {
		select {
		case <-volumeTicker.C:
			// Check if client is connected before publishing
			if app.isClientConnected() {
				app.updateVolume(app.client)
				app.updateMute(app.client)
				app.updateMediaDevices(app.client)
				app.client.Publish(app.getTopicPrefix()+"/status/alive", 0, true, "online")
			} else if networkReachable {
				log.Println("MQTT client not connected but network is reachable, connection may be recovering")
			}

		case <-batteryTicker.C:
			if app.isClientConnected() {
				app.updateBattery(app.client)
				app.updateDiskUsage(app.client)
				app.updateCPUUsage(app.client)
				app.updateMemoryUsage(app.client)
				app.updateUptime(app.client)
				app.updatePublicIP(app.client)
			} else if networkReachable {
				log.Println("MQTT client not connected but network is reachable, skipping battery update")
			}

		case <-awakeTicker.C:
			if app.isClientConnected() {
				app.updateCaffeinateStatus(app.client)
				app.updateDisplayBrightness(app.client)
			} else if networkReachable {
				log.Println("MQTT client not connected but network is reachable, skipping status updates")
			}
			// Note: Media updates now come from the media-control stream

		case <-networkCheckTicker.C:
			// Periodic network reachability check
			currentNetworkState := app.isNetworkReachable()
			currentConnectionState := app.isClientConnected()

			// Log network state changes
			if currentNetworkState != networkReachable {
				if currentNetworkState {
					log.Println("Network connectivity restored - MQTT broker is now reachable")
				} else {
					log.Println("Network connectivity lost - MQTT broker is no longer reachable")
				}
				networkReachable = currentNetworkState
			}

			// Log connection state changes
			if currentConnectionState != lastConnectionState {
				if currentConnectionState {
					log.Println("MQTT connection restored")
				} else {
					log.Println("MQTT connection lost")
				}
				lastConnectionState = currentConnectionState
			}

			// Handle network state changes
			if currentNetworkState && !networkReachable {
				// Network just became reachable - try to reconnect if not already connected
				if !currentConnectionState {
					log.Println("Attempting to reconnect to MQTT broker...")
					// The auto-reconnect should handle this, but we can force a reconnection attempt
					go func() {
						if app.client == nil {
							return
						}
						if token := app.client.Connect(); token.Wait() && token.Error() != nil {
							log.Printf("Reconnection attempt failed: %v", token.Error())
						}
					}()
				}
			}
		}
	}
}

// isClientConnected reports whether the MQTT client exists and is connected.
// During offline mode (broker unreachable at startup) app.client is nil, so
// callers must use this instead of dereferencing app.client directly to avoid
// a nil-pointer panic.
func (app *Application) isClientConnected() bool {
	return app.client != nil && app.client.IsConnected()
}

// Input validation functions

// validateVolumeInput validates volume input (0-100)
func (app *Application) validateVolumeInput(payload string) (int, error) {
	volume, err := strconv.Atoi(payload)
	if err != nil {
		return 0, fmt.Errorf("volume must be a number: %w", err)
	}
	if volume < MinVolume || volume > MaxVolume {
		return 0, fmt.Errorf("volume must be between %d and %d, got %d", MinVolume, MaxVolume, volume)
	}
	return volume, nil
}

// validateMuteInput validates mute input (true/false)
func (app *Application) validateMuteInput(payload string) (bool, error) {
	mute, err := strconv.ParseBool(payload)
	if err != nil {
		return false, fmt.Errorf("mute must be true or false: %w", err)
	}
	return mute, nil
}

// validateBrightnessInput validates brightness input (0-100)
func (app *Application) validateBrightnessInput(payload string) (int, error) {
	brightness, err := strconv.Atoi(payload)
	if err != nil {
		return 0, fmt.Errorf("brightness must be a number: %w", err)
	}
	if brightness < MinBrightness || brightness > MaxBrightness {
		return 0, fmt.Errorf("brightness must be between %d and %d, got %d", MinBrightness, MaxBrightness, brightness)
	}
	return brightness, nil
}

// validateShortcutInput validates shortcut input
func (app *Application) validateShortcutInput(payload string) error {
	if payload == "" {
		return fmt.Errorf("shortcut name cannot be empty")
	}
	// Basic validation - shortcut name should be alphanumeric with spaces and hyphens
	matched, err := regexp.MatchString(`^[a-zA-Z0-9\s\-_]+$`, payload)
	if err != nil {
		return fmt.Errorf("error validating shortcut name: %w", err)
	}
	if !matched {
		return fmt.Errorf("shortcut name contains invalid characters")
	}
	return nil
}

// validateKeepAwakeInput validates keep awake input (true/false)
func (app *Application) validateKeepAwakeInput(payload string) (bool, error) {
	keepAwake, err := strconv.ParseBool(payload)
	if err != nil {
		return false, fmt.Errorf("keep awake must be true or false: %w", err)
	}
	return keepAwake, nil
}

func main() {
	// Create and initialize the application
	app, err := NewApplication()
	if err != nil {
		log.Fatal("Failed to initialize application: ", err)
	}

	// Run the application
	if err := app.Run(); err != nil {
		log.Fatal("Application error: ", err)
	}
}
