package hub

import (
	"collabserver/collabauth"
	"collabserver/storage"
	"context"
	"errors"
	"log"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// The ids for relevant collections/subcollections in Firestore.
	hubsID  = "hubs"
	authID  = "authorization"
	filesID = "files"
	opsID   = "operations"
)

var (
	// ErrorEntryNotFound is given when an entry/document of a given id is not within the provided collection.
	ErrorEntryNotFound = errors.New("could not find the Document")
	// ErrorCollectionNotFound is given when there is an error while retrieving a Collection of a given
	// id from a Document.
	ErrorCollectionNotFound = errors.New("could not find the collection")
	errUnauthorized         = errors.New("user not authorized to perform action")
)

type messageProcessor func(message *Message) *Message
type userConnector func(userID string, hub string) (success bool)
type datastore interface {
	AddEntry(collection *firestore.CollectionRef, id string, data interface{}) (*firestore.DocumentRef, error)
	DocExists(docID string, collection *firestore.CollectionRef) (bool, *firestore.DocumentRef, error)
	UpdateEntry(docRef *firestore.DocumentRef, path string, value interface{}) error
	CommitOps(opsCollection *firestore.CollectionRef, idx int64, ops []string, committerID string) (string, int64, []string, string)
	CollectionForID(collectionID string, docRef *firestore.DocumentRef) *firestore.CollectionRef
}

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

	db datastore

	ref *firestore.DocumentRef

	auth collabauth.Authenticator

	users *firestore.CollectionRef

	files *firestore.CollectionRef
}

// hubRef defines what a hub entry looks like in the hubs collection.
type hubRef struct {
	name string
}

// CreateOrRetrieveHub attempts to fetch the hub from the db, and creates a new one
// if it doesn't exist, with userID as the owner.
func CreateOrRetrieveHub(hubName string, userID string) (*Hub, error) {
	hubs := storage.DB.CollectionForID(hubsID, nil)
	if hubs == nil {
		return nil, ErrorCollectionNotFound
	}
	return newHub(hubName, userID, hubs)
}

// newHub creates a new Hub object for backend use. It creates a corresponding hub Document
// in Firestore if it doesn't already exist.
func newHub(hubName, userID string, hubs *firestore.CollectionRef) (*Hub, error) {
	exists, docRef, err := storage.DB.DocExists(hubName, hubs)
	if !exists {
		if status.Code(err) != codes.NotFound {
			// Some unknown, unhandled error
			return nil, err
		}
		// At this point Create should work (or at least not fail due to Doc already existing),
		// but it's possible there might be some connection error or something.
		err = createHubWithOwner(docRef, hubName, userID)
		if err != nil {
			return nil, err
		}
	}
	hub := &Hub{
		name:       hubName,
		inbound:    make(chan *Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		ref:        docRef,
		db:         storage.DB,
	}
	err = hub.init()
	if err != nil {
		return nil, err
	}
	return hub, nil
}

func createHubWithOwner(docRef *firestore.DocumentRef, hubName, ownerID string) error {
	_, err := docRef.Create(context.Background(), hubRef{name: hubName})
	if err != nil {
		return err
	}
	err = collabauth.AddOwnerToNewHub(ownerID, docRef.Collection(authID))
	return err
}

func (h *Hub) init() error {
	// Get the subcollections auth and file which should always have the same ids
	// across hubs.
	authCollection := h.ref.Collection(authID)
	if authCollection == nil {
		log.Printf("Could not get auth collection for hub %s", h.name)
		return ErrorCollectionNotFound
	}
	fileCollection := h.ref.Collection(filesID)
	if fileCollection == nil {
		log.Printf("Could not get files collection for hub %s", h.name)
		return ErrorCollectionNotFound
	}
	h.auth = collabauth.CurrentAuthenticator(authCollection)
	h.users = authCollection
	h.files = fileCollection
	return nil
}

// Run starts the hub and listens on all channels for messages.
func (h *Hub) Run() {
	log.Printf("start hub: %s", h.name)
	for {
		select {
		case client, ok := <-h.register:
			if !ok {
				return
			}
			// The minimum permissions for hub access is read access.
			if err := h.ConnectUser(client.userID); err != nil {
				log.Printf("User %s does not have permission to access hub %s: %v", client.userID, h.name, err)
				h.closeClient(client)
				break
			}
			h.clients[client] = true
			// TODO(andrezhu@): push message of hub file list
		case client, ok := <-h.unregister:
			if !ok {
				return
			}
			if _, ok := h.clients[client]; ok {
				h.DisconnectUser(client.userID)
				h.closeClient(client)
			}
		case message, ok := <-h.inbound:
			// Auth check is in processMessage.
			if !ok {
				return
			}
			retMessage := h.processMessage(message)
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
	log.Printf("close hub: %s", h.name)
}
