package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var hostname string

type config struct {
	Ip              string `yaml:"mqtt_ip"`
	Port            string `yaml:"mqtt_port"`
	User            string `yaml:"mqtt_user"`
	Password        string `yaml:"mqtt_password"`
	Hostname        string `yaml:"hostname"`
	DiscoveryPrefix string `yaml:"discovery_prefix"`
}

func (c *config) getConfig() *config {

	configContent, err := os.ReadFile("/Users/jeff/mac2mqtt/mac2mqtt.yaml")
	if err != nil {
		log.Fatal("No config file provided")
	}

	err = yaml.Unmarshal(configContent, c)
	if err != nil {
		log.Fatal("No data in config file")
	}

	if c.Ip == "" {
		log.Fatal("Must specify mqtt_ip in mac2mqtt.yaml")
	}

	if c.Port == "" {
		log.Fatal("Must specify mqtt_port in mac2mqtt.yaml")
	}

	if c.User == "" {
		log.Fatal("Must specify mqtt_user in mac2mqtt.yaml")
	}

	if c.Password == "" {
		log.Fatal("Must specify mqtt_password in mac2mqtt.yaml")
	}
	if c.Hostname == "" {
		c.Hostname = getHostname()
	}
	if c.DiscoveryPrefix == "" {
		c.DiscoveryPrefix = "homeassistant"
	}
	return c
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
	if err != nil {
	}
	if output == "missing value" {
		currentsource := getCommandOutput("/opt/homebrew/bin/switchaudiosource", "-c")
		resp := &http.Response{}
		err := error(nil)

		if currentsource == "DELL U3417W" {
			resp, err = http.Get(`http://localhost:55777/get?name=DELL%20U3417W&mute`)
		} else {
			resp, err = http.Get(`http://localhost:55777/get?name=DELL%20U3824DW&mute`)
		}
		if err != nil {
			log.Println("Error getting mute status: " + err.Error())
		}
		defer resp.Body.Close()
		output, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error getting mute status body: " + err.Error())
		}
		output = []byte(strings.TrimSuffix(string(output), "\n"))
		mute := string(output)
		log.Println("Mute Output: " + mute)
		b = mute == "on"
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
		resp := &http.Response{}
		err := error(nil)
		if currentsource == "DELL U3417W" {
			resp, err = http.Get(`http://localhost:55777/get?name=DELL%20U3417W&volume`)
		} else {
			resp, err = http.Get(`http://localhost:55777/get?name=DELL%20U3824DW&volume`)
		}
		if err != nil {
			log.Println("Error getting volume status: " + err.Error())
		}
		defer resp.Body.Close()
		output, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error getting volume status body: " + err.Error())
		}
		output = []byte(strings.TrimSuffix(string(output), "\n"))
		outputStr := string(output)
		log.Println("Vol Output: " + outputStr)
		f, err := strconv.ParseFloat(outputStr, 64)
		if err != nil {
		}
		i = int(f * 100)
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
		if currentsource == "DELL U3417W" {
			http.Get(`http://localhost:55777/set?name=DELL%20U3417W&volume=` + fmt.Sprintf("%f", volumef))
		} else {
			http.Get(`http://localhost:55777/set?name=DELL%20U3824DW&volume=` + fmt.Sprintf("%f", volumef))
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
		if currentsource == "DELL U3417W" {
			http.Get(`http://localhost:55777/set?name=DELL%20U3417W&mute=` + state)
		} else if currentsource == "DELL U3824DW" {
			http.Get(`http://localhost:55777/set?name=DELL%20U3824DW&mute=` + state)
		} else {
			return
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

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic())
	listen(client, msg)
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Println("Connected to MQTT")

	token := client.Publish(getTopicPrefix()+"/status/alive", 0, true, "online")
	token.Wait()

	log.Println("Sending 'online' to topic: " + getTopicPrefix() + "/status/alive")
	sub(client, getTopicPrefix()+"/command/#")

}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Printf("Disconnected from MQTT: %v", err)
}

func getMQTTClient(ip, port, user, password string) mqtt.Client {

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("ssl://%s:%s", ip, port))
	opts.SetUsername(user)
	opts.SetPassword(password)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	opts.SetDefaultPublishHandler(messagePubHandler)

	opts.SetWill(getTopicPrefix()+"/status/alive", "offline", 0, true)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	return client
}

func getTopicPrefix() string {
	return "mac2mqtt/" + hostname
}

func sub(client mqtt.Client, topic string) {
	token := client.Subscribe(topic, 0, nil)
	token.Wait()
	log.Printf("Subscribed to topic: %s\n", topic)
}

func listen(client mqtt.Client, msg mqtt.Message) {
	if msg.Topic() == getTopicPrefix()+"/command/volume" {

		i, err := strconv.Atoi(string(msg.Payload()))
		if err == nil && i >= 0 && i <= 100 {

			setVolume(i)

			updateVolume(client)
			updateMute(client)

		} else {
			log.Println("Incorrect value")
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/mute" {

		b, err := strconv.ParseBool(string(msg.Payload()))
		if err == nil {
			setMute(b)

			updateVolume(client)
			updateMute(client)

		} else {
			log.Println("Incorrect value")
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/sleep" {

		if string(msg.Payload()) == "sleep" {
			commandSleep()
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/displaysleep" {

		if string(msg.Payload()) == "displaysleep" {
			commandDisplaySleep()
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/displaywake" {

		if string(msg.Payload()) == "displaywake" {
			commandDisplayWake()
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/shutdown" {

		if string(msg.Payload()) == "shutdown" {
			commandShutdown()
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/screensaver" {

		if string(msg.Payload()) == "screensaver" {
			commandScreensaver()
		}

	}

	if msg.Topic() == getTopicPrefix()+"/command/runshortcut" {
		commandRunShortcut(string(msg.Payload()))
	}

	if msg.Topic() == getTopicPrefix()+"/command/keepawake" {
		b, err := strconv.ParseBool(string(msg.Payload()))
		if err == nil {
			if b {
				commandKeepAwake()
			} else {
				commandAllowSleep()
			}
			updateCaffeinateStatus(client)
		} else {
			log.Println("Incorrect value")
		}
	}
}

func updateVolume(client mqtt.Client) {
	token := client.Publish(getTopicPrefix()+"/status/volume", 0, false, strconv.Itoa(getCurrentVolume()))
	token.Wait()
}

func updateMute(client mqtt.Client) {
	token := client.Publish(getTopicPrefix()+"/status/mute", 0, false, strconv.FormatBool(getMuteStatus()))
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

func updateBattery(client mqtt.Client) {
	token := client.Publish(getTopicPrefix()+"/status/battery", 0, false, getBatteryChargePercent())
	token.Wait()
}

func updateCaffeinateStatus(client mqtt.Client) {
	token := client.Publish(getTopicPrefix()+"/status/caffeinate", 0, false, strconv.FormatBool(getCaffeinateStatus()))
	token.Wait()
}

func setDevice(client mqtt.Client, DiscoveryPrefix string) {

	keepawake := map[string]interface{}{
		"p":             "switch",
		"name":          "Keep Awake",
		"unique_id":     hostname + "_keepwake",
		"command_topic": getTopicPrefix() + "/command/keepawake",
		"payload_on":    "true",
		"payload_off":   "false",
		"state_topic":   getTopicPrefix() + "/status/caffeinate",
		"icon":          "mdi:coffee",
	}

	displaywake := map[string]interface{}{
		"p":             "button",
		"name":          "Display Wake",
		"unique_id":     hostname + "_displaywake",
		"command_topic": getTopicPrefix() + "/command/displaywake",
		"payload_press": "displaywake",
		"icon":          "mdi:monitor",
	}

	displaysleep := map[string]interface{}{
		"p":             "button",
		"name":          "Display Sleep",
		"unique_id":     hostname + "_displaywake",
		"command_topic": getTopicPrefix() + "/command/displaysleep",
		"payload_press": "displaysleep",
		"icon":          "mdi:monitor-off",
	}

	screensaver := map[string]interface{}{
		"p":             "button",
		"name":          "Screensaver",
		"unique_id":     hostname + "_screensaver",
		"command_topic": getTopicPrefix() + "/command/screensaver",
		"payload_press": "screensaver",
		"icon":          "mdi:monitor-star",
	}

	sleep := map[string]interface{}{
		"p":             "button",
		"name":          "Sleep",
		"unique_id":     hostname + "_sleep",
		"command_topic": getTopicPrefix() + "/command/sleep",
		"payload_press": "sleep",
		"icon":          "mdi:sleep",
	}

	shutdown := map[string]interface{}{
		"p":                  "button",
		"name":               "Shutdown",
		"unique_id":          hostname + "_shutdown",
		"command_topic":      getTopicPrefix() + "/command/shutdown",
		"payload_press":      "shutdown",
		"enabled_by_default": false,
		"icon":               "mdi:power",
	}
	mute := map[string]interface{}{
		"p":             "switch",
		"name":          "Mute",
		"unique_id":     hostname + "_mute",
		"command_topic": getTopicPrefix() + "/command/mute",
		"payload_on":    "true",
		"payload_off":   "false",
		"state_topic":   getTopicPrefix() + "/status/mute",
		"icon":          "mdi:volume-mute",
	}

	volume := map[string]interface{}{
		"p":             "number",
		"name":          "Volume",
		"unique_id":     hostname + "_volume",
		"command_topic": getTopicPrefix() + "/command/volume",
		"state_topic":   getTopicPrefix() + "/status/volume",
		"min_value":     0,
		"max_value":     100,
		"step":          1,
		"mode":          "slider",
		"icon":          "mdi:volume-high",
	}

	battery := map[string]interface{}{
		"p":                   "sensor",
		"name":                "Battery",
		"unique_id":           hostname + "_battery",
		"state_topic":         getTopicPrefix() + "/status/battery",
		"enabled_by_default":  false,
		"unit_of_measurement": "%",
		"device_class":        "battery",
	}

	components := map[string]interface{}{
		"sleep":        sleep,
		"shutdown":     shutdown,
		"volume":       volume,
		"mute":         mute,
		"displaywake":  displaywake,
		"displaysleep": displaysleep,
		"screensaver":  screensaver,
		"battery":      battery,
		"keepawake":    keepawake,
	}

	origin := map[string]interface{}{
		"name": "mac2mqtt",
	}

	device := map[string]interface{}{
		"ids":  getSerialnumber(),
		"name": hostname,
		"mf":   "Apple",
		"mdl":  getModel(),
	}

	object := map[string]interface{}{
		"dev":                device,
		"o":                  origin,
		"cmps":               components,
		"availability_topic": getTopicPrefix() + "/status/alive",
		"qos":                2,
	}
	objectJSON, _ := json.Marshal(object)

	token := client.Publish(DiscoveryPrefix+"/device"+"/"+hostname+"/config", 0, true, objectJSON)
	token.Wait()
}

func main() {

	log.Println("Started")
	var c config
	c.getConfig()
	log.Println("Discovery Prefix: " + c.DiscoveryPrefix)

	hostname = c.Hostname
	var wg sync.WaitGroup
	mqttClient := getMQTTClient(c.Ip, c.Port, c.User, c.Password)

	volumeTicker := time.NewTicker(60 * time.Second)
	batteryTicker := time.NewTicker(60 * time.Second)
	awakeTicker := time.NewTicker(60 * time.Second)
	setDevice(mqttClient, c.DiscoveryPrefix)
	updateVolume(mqttClient)
	updateMute(mqttClient)
	updateCaffeinateStatus(mqttClient)

	wg.Add(1)
	go func() {
		for {
			select {
			case <-volumeTicker.C:
				updateVolume(mqttClient)
				updateMute(mqttClient)

			case <-batteryTicker.C:
				updateBattery(mqttClient)
			case <-awakeTicker.C:
				updateCaffeinateStatus(mqttClient)
			}
		}
	}()

	wg.Wait()

}
