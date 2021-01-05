package collabauth

import (
	"context"
	"log"
	"testing"

	"cloud.google.com/go/firestore"
)

const (
	testHub = "TESTING"
)

var ()

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
	auth.Add(context.Background(), AuthEntry{
		UserID: "owner",
		Hub:    testHub,
		Role:   Owner,
	})

	auth.Add(context.Background(), AuthEntry{
		UserID: "writer",
		Hub:    testHub,
		Role:   Writer,
	})

	auth.Add(context.Background(), AuthEntry{
		UserID: "reader",
		Hub:    testHub,
		Role:   Viewer,
	})

	return &firestoreAuthenticator{
		authTable: auth,
	}
}

func TestVerifyAccess(t *testing.T) {
	client := newFirestoreTestClient(context.Background())
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
				if ok, _ := auth.verifyAccess(tc.userID, testHub, op); ok != tc.expected {
					t.Errorf("verifyAccess gave the wrong access for role %s and operation %s: got %t, want %t",
						tc.userID, op, ok, tc.expected)
				}
			}

		})
	}
}

func TestRoleForUserID(t *testing.T) {
	client := newFirestoreTestClient(context.Background())
	auth := newAuthenticator(client)

	cases := []struct {
		name     string
		hub      string
		userID   string
		expected string
	}{
		{
			name:     "gives correct role",
			hub:      testHub,
			userID:   "owner",
			expected: Owner,
		},
		{
			name:     "nonexistent hub gives no role",
			hub:      "fakeHub",
			userID:   "owner",
			expected: NoRole,
		},
		{
			name:     "nonexistent user gives no role",
			hub:      testHub,
			userID:   "fakeUser",
			expected: NoRole,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual, _ := auth.roleForUserID(tc.hub, tc.userID); actual != tc.expected {
				t.Errorf("roleForUserID, for hub %s, gave role %s but want %s", tc.hub, actual, tc.expected)
			}
		})
	}
}
