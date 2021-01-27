package collabauth

import (
	"collabserver/collections"
	"collabserver/hubcodes"
	"collabserver/storage"
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
)

const (
	// NoRole means the user does not have access to the hub.
	NoRole = "NONE"
	// Viewer means the user has read access to the hub.
	Viewer = "VIEWER"
	// Writer means the user has write access to the hub.
	Writer = "WRITER"
	// Owner means the user has write access to the hub and can add and remove users.
	Owner = "OWNER"

	opRead        = "READ"
	opWrite       = "WRITE"
	opChangeUsers = "CHANGEUSERS"

	// Role refers to the key for user role under our Firestore collection.
	Role = "role"
)

var (
	errHubNotNew = errors.New("Hub isn't new")
)

// Authenticator defines methods for verifying user access levels with an arbitrary backend.
type Authenticator interface {
	CanCommit(userID string) (bool, *firestore.DocumentRef)
	CanCreateDoc(userID string) (bool, *firestore.DocumentRef)
	CanDeleteDoc(userID string) (bool, *firestore.DocumentRef)
	CanRead(userID string) (bool, *firestore.DocumentRef)
	CanChangeUsers(userID string) (bool, *firestore.DocumentRef)
}

// datastore declares the functions that are used for interacting with Firestore
type datastore interface {
	DocExists(docID string, collection *firestore.CollectionRef) (bool, *firestore.DocumentRef, error)
	CollectionIsEmpty(collection *firestore.CollectionRef) bool
}

// firestoreAuthenticator implements the Authenticator interface and uses a Firestore Collection
// as the authentication reference.
type firestoreAuthenticator struct {
	authTable *firestore.CollectionRef
	db        datastore
}

func (fa *firestoreAuthenticator) verifyAccess(userID string, op string) (bool, *firestore.DocumentRef) {
	role, docRef, err := fa.roleForUserID(userID)
	if err != nil {
		// TODO(itsazhuhere@): Consider also returning an error.
		return false, docRef
	}
	var ok bool
	switch op {
	case opRead:
		ok = role == Viewer || role == Writer || role == Owner
	case opWrite:
		ok = role == Writer || role == Owner
	case opChangeUsers:
		ok = role == Owner
	default:
		fmt.Printf("Unsupported operation: %s", op)
	}
	if !ok {
		docRef = nil
	}
	return ok, docRef
}

func (fa *firestoreAuthenticator) roleForUserID(userID string) (string, *firestore.DocumentRef, error) {
	exists, docRef, err := fa.db.DocExists(userID, fa.authTable)
	if err != nil {
		return NoRole, nil, err
	}
	if !exists {
		// Not really an error, just that the user doesn't belong to the hub.
		return NoRole, nil, nil
	}
	data := &collections.AuthEntry{}
	snapshot, err := docRef.Get(context.Background())
	if err != nil {
		return NoRole, nil, err
	}
	snapshot.DataTo(data)
	role := data.Role
	return role, docRef, nil
}

func (fa *firestoreAuthenticator) CanCommit(userID string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, opWrite)
}

func (fa *firestoreAuthenticator) CanCreateDoc(userID string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, opWrite)
}

func (fa *firestoreAuthenticator) CanDeleteDoc(userID string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, opWrite)
}

func (fa *firestoreAuthenticator) CanRead(userID string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, opRead)
}

func (fa *firestoreAuthenticator) CanChangeUsers(userID string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, opChangeUsers)
}

// AddOwnerToNewHub checks if the collection is empty (indicating a new hub) and adds ownerID as owner.
func AddOwnerToNewHub(ownerID string, collection *firestore.CollectionRef) error {
	if !storage.DB.CollectionIsEmpty(collection) {
		return errHubNotNew
	}
	_, err := storage.DB.AddEntry(collection, ownerID, collections.AuthEntry{
		UserID: ownerID,
		Role:   Owner,
		Status: hubcodes.UserOffline,
	})
	return err
}

// UserIDFromToken grabs the client from the Firebase App and decodes the idToken.
func UserIDFromToken(ctx context.Context, app *firebase.App, idToken string) (string, error) {
	client, err := app.Auth(ctx)
	if err != nil {
		return "", err
	}

	token, err := client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", err
	}

	return token.UID, nil
}

// CurrentAuthenticator gives the currently used authenticator.
func CurrentAuthenticator(authTable *firestore.CollectionRef) Authenticator {
	return &firestoreAuthenticator{
		authTable: authTable,
		db:        storage.DB,
	}
}
