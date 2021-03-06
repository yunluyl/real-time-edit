// Package collections contains data structures and constants relating to Firestore collections and their entry
// structures/keys/values, as well as structs that define what is returned to clients.
package collections

// AuthEntry represents an entry in the our Firestore authorization collection.
type AuthEntry struct {
	UserID string `firestore:"userID"`
	Role   string `firestore:"role"`
	// Status is the online status of the user (i.e. "Online" or "Offline").
	Status string `firestore:"status"`
}

// UserEntry is used in the users collection and is primarily used for associating
// an email to a userID (because Google accounts don't have usernames).
// TODO(itsazhuhere@): can maybe add First and Last names like in Google Drive.
type UserEntry struct {
	UserID string `firestore:"userID"`
	Email  string `firestore:"email"`
}

// UserInfo contains all relevant data about a user in reference to the hub that the initial
// request came from (this usually means user email and role).
type UserInfo struct {
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

// FileInfo contains info on a file within a hub.
type FileInfo struct {
	Name            string       `json:"name" firestore:"name"`
	Deleted         bool         `firestore:"deleted"`
	Snapshot        FileSnapshot `json:"snapshot" firestore:"snapshot"`
	MarkedForUpdate bool         `json:"needsUpdate" firestore:"snapshotNeedsUpdate"`
}

// FileSnapshot holds a snapshot of a notebook file up to operation Index
type FileSnapshot struct {
	// The structure of the file as a JSON parsable string.
	File string `json:"file"`
	// The operation index of the file structure
	Index int `json:"index"`
}

// UserToHubEntry lists the fields of a document in the userToHub collection, which allows easy
// lookup of users' hub memberships.
type UserToHubEntry struct {
	UserID string `firestore:"userID"`
	Hub    string `firestore:"hub"`
	Role   string `firestore:"role"`
}
