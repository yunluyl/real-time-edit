package hub

import (
	"collabserver/collections"
	testutils "collabserver/testing"
	"context"
	"testing"

	"cloud.google.com/go/firestore"
)

type fakeDatastore struct {
	connectUserResult bool
}

func (fd *fakeDatastore) AddEntry(collection *firestore.CollectionRef, id string, data interface{}) (*firestore.DocumentRef, error) {
	return nil, nil
}

func (fd *fakeDatastore) DocExists(docID string, collection *firestore.CollectionRef) (bool, *firestore.DocumentRef, error) {
	return true, nil, nil
}
func (fd *fakeDatastore) UpdateEntry(docRef *firestore.DocumentRef, path string, value interface{}) error {
	return nil
}
func (fd *fakeDatastore) CommitOps(opsCollection *firestore.CollectionRef, idx int64, ops []string, committerID string) (string, int64, []string, string) {
	return "", 0, nil, ""
}
func (fd *fakeDatastore) CollectionForID(collectionID string, docRef *firestore.DocumentRef) *firestore.CollectionRef {
	return nil
}

func (fd *fakeDatastore) AllUsers(collection *firestore.CollectionRef) ([]collections.UserInfo, error) {
	return nil, nil
}

func (fd *fakeDatastore) AllFiles(collection *firestore.CollectionRef) ([]collections.FileInfo, error) {
	return nil, nil
}

func (fd *fakeDatastore) EntryForFieldValue(collection *firestore.CollectionRef, fieldPath string, value, dataTo interface{}) (*firestore.DocumentRef, error) {
	return nil, nil
}

func (fd *fakeDatastore) UserIDsForEmails(emails []string) (map[string]string, error) {
	return nil, nil
}

func fakeProcessMessage(message *Message) *Message {
	return message
}

func TestHub(t *testing.T) {
	ownerID := "owner"
	fakeDb := &fakeDatastore{
		connectUserResult: true,
	}
	client := testutils.NewFirestoreTestClient(context.Background())
	fakeCollection := client.Collection("test_authentication")
	testHub, err := newHub("TESTING", ownerID, fakeCollection)
	if err != nil {
		t.Errorf("newHub gave error: %v when not expecting one.", err)
	}
	// inject the faked storage so we don't actually interact with Firestore.
	testHub.db = fakeDb
	go testHub.Run()

	// Push a client first so the client list isn't empty (which causes the hub to close).
	acceptClient := &Client{
		send:   make(chan *Message),
		userID: ownerID,
	}
	testHub.register <- acceptClient

	// Create a new client but with an unauthorized userID (empty userID works).
	rejectClient := &Client{send: make(chan *Message)}
	testHub.register <- rejectClient
	if _, ok := <-rejectClient.send; ok {
		t.Error("Hub did not reject client when expecting a rejection.")
	}

	fakeText := "test"
	fakeMessage := &Message{
		Text:     fakeText,
		Endpoint: endpointPassthrough,
		Route:    []string{routeBroadcast},
		client:   acceptClient,
	}
	testHub.inbound <- fakeMessage

	var message *Message
	var ok bool
	if message, ok = <-acceptClient.send; !ok {
		t.Error("Hub closed connection when expecting successful message.")
	}
	if message.Text != fakeText {
		t.Errorf("Hub gave unexpected message %v but want %v", message, fakeMessage)
	}

	// Finally test the unregister
	testHub.unregister <- acceptClient
	if _, ok := <-acceptClient.send; ok {
		t.Error("Hub did not close connection when expected to.")
	}
}
