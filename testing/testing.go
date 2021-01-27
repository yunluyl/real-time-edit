package testing

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
)

// NewFirestoreTestClient creates a new client for testing. It requires a local Firestore to be running
// on the user's machine.
func NewFirestoreTestClient(ctx context.Context) *firestore.Client {
	client, err := firestore.NewClient(ctx, "test")
	if err != nil {
		log.Fatalf("firebase.NewClient err: %v", err)
	}

	return client
}
