package main

import (
	"encoding/json"
	"github.com/aeden/eventbus"
	"io/ioutil"
	"log"
	"net"
	"os"
)

var (
	fileServerHost     = os.Getenv("HTTP_FILE_SERVER_HOST")
	fileServerPort     = os.Getenv("HTTP_FILE_SERVER_PORT")
	eventBusServerHost = os.Getenv("HTTP_EVENTBUS_SERVER_HOST")
	eventBusServerPort = os.Getenv("HTTP_EVENTBUS_SERVER_PORT")
)

func loadServicesConfig() *eventbus.ServicesConfig {
	file, e := ioutil.ReadFile("services.json")
	if e != nil {
		log.Printf("Error reading services config: %s", e)
	}

	servicesConfig := eventbus.ServicesConfig{}
	json.Unmarshal(file, &servicesConfig.Services)
	return &servicesConfig
}

func main() {
	servicesConfig := loadServicesConfig()

	eventStore := eventbus.NewInMemoryEventStore()

	fileServerHostAndPort := net.JoinHostPort(fileServerHost, fileServerPort)
	eventBusHostAndPort := net.JoinHostPort(eventBusServerHost, eventBusServerPort)

	eventbus.StartWebsocketHub()

	go eventbus.StartFileServer(fileServerHostAndPort, eventBusHostAndPort)
	eventbus.StartEventBusServer(eventBusHostAndPort, fileServerHostAndPort, servicesConfig, eventStore)

}
