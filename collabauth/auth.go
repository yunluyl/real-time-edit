package collabauth

import (
	"context"
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

// Authenticator defines methods for verifying user access levels with an arbitrary backend.
type Authenticator interface {
	CanCommit(userID string, hub string) (bool, *firestore.DocumentRef)
	CanCreateDoc(userID string, hub string) (bool, *firestore.DocumentRef)
	CanDeleteDoc(userID string, hub string) (bool, *firestore.DocumentRef)
	CanRead(userID string, hub string) (bool, *firestore.DocumentRef)
	CanChangeUsers(userID string, hub string) (bool, *firestore.DocumentRef)
	HubOwners(hub string) []string
	CreateEntry(userID string, hub string) (*firestore.DocumentRef, error)
}

// AuthEntry represents an entry in the our Firestore authorization collection.
type AuthEntry struct {
	UserID string `firestore:"userID"`
	Hub    string `firestore:"hub"`
	Role   string `firestore:"role"`
}

// firestoreAuthenticator implements the Authenticator interface and uses a Firestore Collection
// as the authentication reference.
type firestoreAuthenticator struct {
	authTable *firestore.CollectionRef
}

func (fa *firestoreAuthenticator) verifyAccess(userID string, hub string, op string) (bool, *firestore.DocumentRef) {
	role, docRef := fa.roleForUserID(hub, userID)
	var ok bool
	switch op {
	case opRead:
		ok = role == Viewer || role == Writer || role == Owner
	case opWrite:
		ok = role == Writer || role == Owner
	case opChangeUsers:
		ok = !fa.hubHasOwners(hub) || role == Owner
	default:
		fmt.Printf("Unsupported operation: %s", op)
	}
	if !ok {
		docRef = nil
	}
	return ok, docRef
}

func (fa *firestoreAuthenticator) roleForUserID(hub string, userID string) (string, *firestore.DocumentRef) {
	doc, err := fa.authTable.
		Where("userID", "==", userID).
		Where("hub", "==", hub).
		Documents(context.Background()).
		Next()
	if err == nil {
		data := &AuthEntry{}
		doc.DataTo(data)
		role := data.Role
		return role, doc.Ref
	}
	return NoRole, nil
}

func (fa *firestoreAuthenticator) HubOwners(hub string) []string {
	owners := []string{}
	iter := fa.authTable.
		Where("hub", "==", hub).
		Where("role", "==", Owner).
		Documents(context.Background())
	docs, err := iter.GetAll()
	if err != nil {
		fmt.Printf("Error getting owners for hub %s: %v", hub, err)
	}
	for i := 0; i < len(docs); i++ {
		data := AuthEntry{}
		docs[i].DataTo(data)
		owners = append(owners, data.UserID)
	}
	return owners
}

func (fa *firestoreAuthenticator) hubHasOwners(hub string) bool {
	return len(fa.HubOwners(hub)) > 0
}

func (fa *firestoreAuthenticator) CanCommit(userID string, hub string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, hub, opWrite)
}

func (fa *firestoreAuthenticator) CanCreateDoc(userID string, hub string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, hub, opWrite)
}

func (fa *firestoreAuthenticator) CanDeleteDoc(userID string, hub string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, hub, opWrite)
}

func (fa *firestoreAuthenticator) CanRead(userID string, hub string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, hub, opRead)
}

func (fa *firestoreAuthenticator) CanChangeUsers(userID string, hub string) (bool, *firestore.DocumentRef) {
	return fa.verifyAccess(userID, hub, opChangeUsers)
}

func (fa *firestoreAuthenticator) CreateEntry(userID string, hub string) (*firestore.DocumentRef, error) {
	docRef, _, err := fa.authTable.Add(context.Background(), AuthEntry{
		UserID: userID,
		Hub:    hub,
		Role:   NoRole,
	})
	return docRef, err
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
	}
}
