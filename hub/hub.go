package hub

import (
	log "collabserver/cloudlog"
	"collabserver/collabauth"
	"collabserver/collections"
	"collabserver/storage"
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	// The ids for relevant collections/subcollections in Firestore.
	hubsID        = "hubs"
	authID        = "authorization"
	filesID       = "files"
	opsID         = "operations"
	usersToHubsID = "usersToHubs"

	updateInterval = 2
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
	DeleteDocument(docRef *firestore.DocumentRef) error
	CollectionForID(collectionID string, docRef *firestore.DocumentRef) *firestore.CollectionRef
	AllUsers(collection *firestore.CollectionRef) ([]collections.UserInfo, error)
	AllFiles(collection *firestore.CollectionRef) ([]collections.FileInfo, error)
	UserIDsForEmails(emails []string) (map[string]string, error)
	EntryForFieldValue(collection *firestore.CollectionRef, fieldPath string, value, dataTo interface{}) (*firestore.DocumentRef, error)
	AllHubsForUser(userID string) []string
	UpdateUsersHubList(userID, hubName, role string) error
}

// Hub maintains the set of active clients and send messages to the
// clients based on processor rules.
type Hub struct {
	// Name of this hub
	name string

	// set to true if hub has been closed; can't be reused and needs to be disposed of.
	isClosed bool

	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	inbound chan *Message

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	clientReturn map[*Client]chan *Client

	stopClientSend map[*Client]chan struct{}

	stopPeriodicUpdates chan int

	db datastore

	ref *firestore.DocumentRef

	auth collabauth.Authenticator

	users *firestore.CollectionRef

	files *firestore.CollectionRef

	masterUsersList *firestore.CollectionRef
}

// hubRef defines what a hub entry looks like in the hubs collection.
type hubRef struct {
	name string
}

// CreateOrRetrieveHub attempts to fetch the hub from the db, and creates a new one
// if it doesn't exist, with userID as the owner.
func CreateOrRetrieveHub(hubName string, userID string) (*Hub, error) {
	log.Printf("getting hub %s ", hubName)
	hubs := storage.DB.CollectionForID(hubsID, nil)
	if hubs == nil {
		log.Print("not found")
		return nil, ErrorCollectionNotFound
	}
	return newHub(hubName, userID, hubs)
}

// newHub creates a new Hub object for backend use. It creates a corresponding hub Document
// in Firestore if it doesn't already exist.
func newHub(hubName, userID string, hubs *firestore.CollectionRef) (*Hub, error) {
	exists, docRef, err := storage.DB.DocExists(hubName, hubs)
	if !exists {
		if err != nil {
			// Some unknown, unhandled error; DocExists silences the NotFound error.
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
		name:            hubName,
		isClosed:        false,
		inbound:         make(chan *Message),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		clients:         make(map[*Client]bool),
		stopClientSend:  make(map[*Client]chan struct{}),
		ref:             docRef,
		db:              storage.DB,
		masterUsersList: storage.DB.TopLevelCollection(usersToHubsID),
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

	// Also add them to a collection that allows easy lookup of the hubs a user is a part of.
	masterUsersList := storage.DB.TopLevelCollection(usersToHubsID)
	storage.DB.AddEntry(masterUsersList, "", collections.UserToHubEntry{
		UserID: ownerID,
		Hub:    hubName,
		Role:   collabauth.Owner,
	})

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

	h.clientReturn = make(map[*Client]chan *Client)

	go h.startPeriodicUpdates()

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

			h.stopClientSend[client] = make(chan struct{})
			client.assignChans(h.inbound, h.stopClientSend[client])
			// The minimum permissions for hub access is read access.
			if err := h.ConnectUser(client.userID); err != nil {
				log.Printf("User %s does not have permission to access hub %s: %v", client.userID, h.name, err)
				h.closeClient(client)
				break
			}
			h.clients[client] = true
			h.sendMessage(client, h.hubConnectSuccessMessage(client))
		case client, ok := <-h.unregister:
			if !ok {
				return
			}
			log.Print("returning client to connector")
			if _, ok := h.clients[client]; ok {
				h.DisconnectUser(client.userID)
				h.clientReturn[client] <- client
				delete(h.clientReturn, client)
				h.removeClient(client)
			}
		case message, ok := <-h.inbound:
			// Auth check is in processMessage.
			if !ok {
				return
			}
			retMessage := h.processMessage(message)
			h.handleSendMessage(retMessage, message.client)
		}
	}
}

func (h *Hub) registerUser(client *Client, returnClient chan *Client) {
	if h.IsClosed() {
		log.Print("register client failed because hub is closed")
		return
	}
	log.Printf("registering client: %#v to hub: %#v", client, h)
	h.clientReturn[client] = returnClient
	h.register <- client
}

func (h *Hub) handleSendMessage(message *Message, origin *Client) {
	if message == nil {
		// No op
		return
	}
	if len(message.Route) > 0 {
		if message.Route[0] == routeBroadcast {
			for client := range h.clients {
				h.sendMessage(client, message)
			}
		} else if message.Route[0] == routeOrigin {
			h.sendMessage(origin, message)
		} else {
			routes := make(map[string]bool)
			for _, dest := range message.Route {
				routes[dest] = true
			}
			for client := range h.clients {
				if _, ok := routes[client.userID]; ok {
					h.sendMessage(client, message)
				}
			}
		}
	}
}

func (h *Hub) sendMessage(client *Client, message *Message) {
	if client == nil {
		return
	}
	if client.IsClosed() {
		h.closeClient(client)
		return
	}
	select {
	case client.send <- message:
	default:
		h.closeClient(client)
	}
}

func (h *Hub) startPeriodicUpdates() {
	ticker := time.NewTicker(updateInterval * time.Second)
	h.stopPeriodicUpdates = make(chan int)
	for {
		select {
		case <-ticker.C:
			users, err := h.allUsers()
			if err != nil {
				log.Printf("Error getting all users of hub %s", h.name)
				break
			}
			message := &Message{
				Endpoint: endpointListUsers,
				Route: []string{
					routeBroadcast,
				},
				UserList: users,
			}

			h.handleSendMessage(message, nil)
		case <-h.stopPeriodicUpdates:
			return
		}
	}
}

// IsClosed determines if a hub has been closed or not. Useful for maintaining a list of hubs
// that may or may not need to be closed as needed.
func (h *Hub) IsClosed() bool {
	return h.isClosed
}

// hands the client back to the hub connector.
func (h *Hub) handBackClient(client *Client) {
	h.unregister <- client
}

func (h *Hub) closeClient(client *Client) {
	h.removeClient(client)
}

func (h *Hub) removeClient(client *Client) {
	close(h.stopClientSend[client])
	delete(h.stopClientSend, client)
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
	go func() {
		// This can hang depending on where in the call stack closeHub is called.
		// Doing this in a goroutine means functions can resolve and avoid the hang.
		h.stopPeriodicUpdates <- 0
	}()
	h.isClosed = true
	log.Printf("close hub: %s", h.name)
}
