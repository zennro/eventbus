package eventbus

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

type wsConnection struct {
	// The websocket connection.
	ws *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

type wsHub struct {
	// Registered connections.
	connections map[*wsConnection]*wsConnectionState

	// Inbound messages from the connections.
	broadcast chan []byte

	// Actions from connections
	execute chan *wsCommand

	// Register requests from the connections.
	register chan *wsConnection

	// Unregister requests from connections.
	unregister chan *wsConnection
}

type wsCommand struct {
	source *wsConnection
	Action string `json:"action"`
}

type wsIdentifyCommandResponse struct {
	Action string `json:"action"`
	Token  string `json:"token"`
}

type wsAuthenticateCommandResponse struct {
	Action string `json:"action"`
}

type wsCommandErrorResponse struct {
	Action       string `json:"action"`
	ErrorMessage string `json:"error"`
}

type wsConnectionState struct {
	Token      string
	ClientType string
}

var WebsocketHub = wsHub{
	broadcast:   make(chan []byte),
	execute:     make(chan *wsCommand),
	register:    make(chan *wsConnection),
	unregister:  make(chan *wsConnection),
	connections: make(map[*wsConnection]*wsConnectionState),
}

// Broadcast the given message to all listeners
func (h *wsHub) Send(message []byte) {
	h.broadcast <- message
}

// Send the message to a specific client
func (h *wsHub) SendToClient(clientAccessToken string, message []byte) {
	for c, cs := range h.connections {
		if cs.Token == clientAccessToken {
			h.sendToConnection(c, message)
			return
		}
	}
}

// Send the message to all services
func (h *wsHub) SendToServices(message []byte) {
	for c, cs := range h.connections {
		if cs.ClientType == "service" {
			h.sendToConnection(c, message)
		}
	}
}

// Send the message to the specific connection
func (h *wsHub) sendToConnection(c *wsConnection, m []byte) {
	select {
	case c.send <- m:
	default:
		delete(h.connections, c)
		close(c.send)
	}
}

func (h *wsHub) run() {
	log.Printf("Run websocket hub")
	for {
		select {
		case connection := <-h.register:
			log.Printf("Register connection %s", connection.ws.RemoteAddr())
			h.connections[connection] = &wsConnectionState{}
		case connection := <-h.unregister:
			log.Printf("Unregister connection %s", connection.ws.RemoteAddr())
			if _, ok := h.connections[connection]; ok {
				delete(h.connections, connection)
				close(connection.send)
			}
		case message := <-h.broadcast:
			for connection := range h.connections {
				h.sendToConnection(connection, message)
			}
		case command := <-h.execute:
			execute(command)
		}
	}
}

func execute(command *wsCommand) {
	if command.Action == "identify" {
		connectionState := WebsocketHub.connections[command.source]
		connectionState.Token = randSeq(60)
		connectionState.ClientType = "client"

		response := &wsIdentifyCommandResponse{
			Action: command.Action,
			Token:  connectionState.Token,
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			log.Printf("Error marshaling response JSON: %s", err)
			return
		}

		WebsocketHub.sendToConnection(command.source, responseJSON)
	} else if command.Action == "authenticate" {
		connectionState := WebsocketHub.connections[command.source]
		connectionState.ClientType = "service"

		response := &wsAuthenticateCommandResponse{
			Action: command.Action,
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			log.Printf("Error marshaling response JSON: %s", err)
			return
		}
		WebsocketHub.sendToConnection(command.source, responseJSON)
	} else {
		response := &wsCommandErrorResponse{
			Action:       command.Action,
			ErrorMessage: "Unknown command",
		}
		responseJSON, err := json.Marshal(response)
		if err != nil {
			log.Printf("Error marshaling response JSON: %s", err)
			return
		}
		WebsocketHub.sendToConnection(command.source, responseJSON)
	}
}

func (c *wsConnection) reader() {
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from web socket: %s", err)
			break
		}
		log.Printf("Message received: %s", message)

		command := &wsCommand{source: c}
		json.Unmarshal(message, command)
		log.Printf("Command received: %s", command.Action)

		WebsocketHub.execute <- command
	}
	c.ws.Close()
}

func (c *wsConnection) writer() {
	for message := range c.send {
		err := c.ws.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
	c.ws.Close()
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

func WebsocketHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %s", err)
		return
	}
	log.Printf("Establishing websocket connection")
	c := &wsConnection{send: make(chan []byte, 256), ws: ws}
	WebsocketHub.register <- c
	defer func() { WebsocketHub.unregister <- c }()
	go c.writer()
	c.reader()
}

func StartWebsocketHub() {
	go WebsocketHub.run()
}
