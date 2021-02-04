package hub

import (
	log "collabserver/cloudlog"
	"collabserver/storage"
	"collabserver/websocketcodes"
	"math/rand"
	"net/http"
)

const (
	hubCodeLength             = 6
	letters                   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	maxCodeGenerationAttempts = 10
)

// Connector facilitates connecting users to a hub.
type Connector struct {
	hubs map[string]*Hub

	db datastore

	clientQueue chan *Client
}

func (hc *Connector) init() {
	hc.hubs = map[string]*Hub{}
	hc.db = storage.DB

	hc.clientQueue = make(chan *Client)
	go hc.ReceiveBackClients()
}

// GetOrRetrieve looks for the hub in the database and creates a new entry if it doesn't exist,
// returning the result either way.
func (hc *Connector) GetOrRetrieve(hubName, userID string) (*Hub, error) {
	currentHub, ok := hc.hubs[hubName]
	if !ok || currentHub.IsClosed() {
		var err error
		currentHub, err = CreateOrRetrieveHub(hubName, userID)
		if err != nil {
			log.Printf("Error creating hub %s: %v", hubName, err)
			return nil, err
		}

		hc.hubs[hubName] = currentHub
		go currentHub.Run()
	}

	return currentHub, nil
}

// RetrieveHubList gives a list of hubs that user userID can access.
func (hc *Connector) RetrieveHubList(userID string) []string {
	return hc.db.AllHubsForUser(userID)
}

// ReceiveBackClients waits for clients to exit out of their hubs and handles their requests until they
// connect back to a hub.
func (hc *Connector) ReceiveBackClients() {
	for {
		select {
		case client, ok := <-hc.clientQueue:
			if !ok {
				log.Print("Client return queue closed (shouldn't happen), exiting")
				return
			}

			go hc.respondUntilHandoff(client)
		}
	}
}

// ServeWs handles the websocket connection and responds to the messages from the client until it connects to a hub.
func (hc *Connector) ServeWs(userID string, w http.ResponseWriter, r *http.Request, response http.Header) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	conn, err := upgrader.Upgrade(w, r, response)
	if err != nil {
		log.Println(err)
		return
	}

	client := NewClient(userID, conn)
	client.Start()
	go hc.respondUntilHandoff(client)
}

// Responds to client messages (currently only supporting hub list requests) until the client connects to a hub.
func (hc *Connector) respondUntilHandoff(client *Client) {
	tempMessageReceiver := make(chan *Message)
	stopClientSend := make(chan struct{})
	defer func() {
		close(tempMessageReceiver)
		close(stopClientSend)
	}()
	client.assignChans(tempMessageReceiver, stopClientSend)
	for {
		msg, ok := <-tempMessageReceiver
		if !ok {
			log.Print("channel to client has been closed in hub connector")
			return
		}
		var returnMessage *Message
		switch msg.Endpoint {
		case endpointListHub:
			hubList := hc.RetrieveHubList(client.userID)
			returnMessage = toOriginWithStatus(msg, websocketcodes.StatusSuccess, "ok")
			returnMessage.HubList = hubList
		case endpointConnectToHub:
			hub, err := hc.GetOrRetrieve(msg.HubName, client.userID)
			if err != nil {
				returnMessage = toOriginWithStatus(msg, websocketcodes.StatusFailure, "failed to retrieve hub")
			} else {
				hub.registerUser(client, hc.clientQueue)
				return
			}
		case endpointHubCreate:
			for i := 0; i < maxCodeGenerationAttempts; i++ {
				hubName := generateRandomHubCode(hubCodeLength)
				hub, err := hc.GetOrRetrieve(hubName, client.userID)
				if err != nil {
					log.Printf("Error while generating new hub code: %#v", err.Error())
					continue
				}
				hub.registerUser(client, hc.clientQueue)
				return
			}
			// If we're here, we continued every loop and failed to make a hub
			returnMessage = toOriginWithStatus(msg, websocketcodes.StatusFailure, "failed to create hub")
		default:
			returnMessage = toOriginWithStatus(msg, websocketcodes.StatusNotConnectedToHub, "Connect to a hub first")
		}
		client.send <- returnMessage
	}
}

// NewConnector returns an instantiated HubConnector
func NewConnector() *Connector {
	hubconnector := &Connector{}
	hubconnector.init()
	return hubconnector
}

func generateRandomHubCode(n int) string {
	runes := make([]byte, n)
	for i := range runes {
		runes[i] = letters[rand.Intn(len(letters))]
	}
	return string(runes)

}
