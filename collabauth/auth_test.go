package collabauth

import (
	log "collabserver/cloudlog"
	"collabserver/collections"
	"collabserver/storage"
	testutils "collabserver/testing"
	"context"
	"testing"

	"cloud.google.com/go/firestore"
)

func newFirestoreTestClient(ctx context.Context) *firestore.Client {
	client, err := firestore.NewClient(ctx, "test")
	if err != nil {
		log.Fatalf("firebase.NewClient err: %v", err)
	}

	return client
}

// newAuthenticator creates a table with at least one of each role for testing purposes.
func newAuthenticator(client *firestore.Client) *firestoreAuthenticator {
	// populate the table
	auth := client.Collection("test_authentication")
	auth.Doc("owner").Set(context.Background(), collections.AuthEntry{
		UserID: "owner",
		Role:   Owner,
	})

	auth.Doc("writer").Set(context.Background(), collections.AuthEntry{
		UserID: "writer",
		Role:   Writer,
	})

	auth.Doc("reader").Set(context.Background(), collections.AuthEntry{
		UserID: "reader",
		Role:   Viewer,
	})

	return &firestoreAuthenticator{
		authTable: auth,
		db:        storage.DB,
	}
}

func TestVerifyAccess(t *testing.T) {
	client := testutils.NewFirestoreTestClient(context.Background())
	auth := newAuthenticator(client)

	cases := []struct {
		name     string
		userID   string
		ops      []string
		expected bool
	}{
		{
			name:   "owner can do all",
			userID: "owner",
			ops: []string{
				opChangeUsers,
				opRead,
				opWrite,
			},
			expected: true,
		},
		{
			name:   "writer can write and read",
			userID: "writer",
			ops: []string{
				opRead,
				opWrite,
			},
			expected: true,
		},
		{
			name:   "reader can do read",
			userID: "reader",
			ops: []string{
				opRead,
			},
			expected: true,
		},
		{
			name:   "writer can't change users",
			userID: "writer",
			ops: []string{
				opChangeUsers,
			},
			expected: false,
		},
		{
			name:   "reader can't change users or write",
			userID: "reader",
			ops: []string{
				opChangeUsers,
				opWrite,
			},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, op := range tc.ops {
				if ok, _ := auth.verifyAccess(tc.userID, op); ok != tc.expected {
					t.Errorf("verifyAccess gave the wrong access for role %s and operation %s: got %t, want %t",
						tc.userID, op, ok, tc.expected)
				}
			}

		})
	}
}

func TestRoleForUserID(t *testing.T) {
	client := testutils.NewFirestoreTestClient(context.Background())
	auth := newAuthenticator(client)

	cases := []struct {
		name     string
		userID   string
		expected string
		hasError bool
	}{
		{
			name:     "gives correct role",
			userID:   "owner",
			expected: Owner,
		},
		{
			name:     "nonexistent user gives no role",
			userID:   "fakeUser",
			expected: NoRole,
			hasError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, _, err := auth.roleForUserID(tc.userID)
			hasError := err != nil
			if hasError != tc.hasError {
				// For saying if we did or did not expect an error to be returned.
				var descriptionString string
				if tc.hasError {
					descriptionString = "not"
				}
				t.Errorf("roleForUserID gave unexpected error response %v when expecting to %s have error", err, descriptionString)
			}
			if actual != tc.expected {
				t.Errorf("roleForUserID gave role %s but want %s", actual, tc.expected)
			}
		})
	}
}
