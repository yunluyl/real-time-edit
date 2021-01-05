package main

import (
	"log"
)

// Hub maintains the set of active clients and send messages to the
// clients based on processor rules.
type Hub struct {
	// Name of this hub
	name string

	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	inbound chan *Message

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func createOrRetrieveHub(hubName string, userID string) (*Hub, error) {
	// If !exists then create and register owner user
	// Doing this here means that we will avoid a situation where a hub doesn't have an owner.
	if len(db.hubOwners(hubName)) < 1 {
		log.Printf("Creating hub %s in db", hubName)
		err := db.createHub(userID, hubName)
		if err != nil {
			return nil, err
		}
	}
	// Finally create backend reference to the hub.
	return newHub(hubName), nil
}

func newHub(name string) *Hub {
	return &Hub{
		name:       name,
		inbound:    make(chan *Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	log.Printf("start hub: %s", h.name)
	for {
		select {
		case client, ok := <-h.register:
			if !ok {
				return
			}
			// The minimum permissions for hub access is read access
			if !db.connectUser(client.userID, h.name) {
				log.Printf("User %s does not have permission to access hub %s", client.userID, h.name)
				h.closeClient(client)
				return
			}
			h.clients[client] = true
			// TODO(andrezhu@): push message of hub file list
		case client, ok := <-h.unregister:
			if !ok {
				return
			}
			if _, ok := h.clients[client]; ok {
				h.closeClient(client)
			}
		case message, ok := <-h.inbound:
			// Auth check is in processMessage.
			if !ok {
				return
			}
			retMessage := processMessage(message)
			if len(retMessage.Route) > 0 {
				if retMessage.Route[0] == routeBroadcast {
					for client := range h.clients {
						h.sendMessage(client, retMessage)
					}
				} else if retMessage.Route[0] == routeOrigin {
					h.sendMessage(message.client, retMessage)
				} else {
					routes := make(map[string]bool)
					for _, dest := range retMessage.Route {
						routes[dest] = true
					}
					for client := range h.clients {
						if _, ok := routes[client.userID]; ok {
							h.sendMessage(client, retMessage)
						}
					}
				}
			}
		}
	}
}

func (h *Hub) sendMessage(client *Client, message *Message) {
	select {
	case client.send <- message:
	default:
		h.closeClient(client)
	}
}

func (h *Hub) closeClient(client *Client) {
	log.Printf("closing client: %+v", client)
	close(client.send)
	delete(h.clients, client)
	if len(h.clients) == 0 {
		h.closeHub()
	}
}

func (h *Hub) closeHub() {
	for client := range h.clients {
		h.closeClient(client)
	}
	close(h.register)
	close(h.unregister)
	close(h.inbound)
	delete(hubs, h.name)
	log.Printf("close hub: %s", h.name)
}
