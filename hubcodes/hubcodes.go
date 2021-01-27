package hubcodes

const (

	// UserOnline is used for when a user is currently logged in an viewing a hub
	UserOnline = "ONLINE"

	// UserOffline is used for when a user is currently not viewing a hub but is still a part of that hub.
	UserOffline = "OFFLINE"

	// UserStatusKey is the key for the status field of a user in our Firestore users collection
	UserStatusKey = "status"

	// UserIDKey is the standard key for most user ID columns in our Firestore collections.
	UserIDKey = "userID"

	// UserEmailKey is the standard key for most email columns in our Firestore collections.
	UserEmailKey = "email"

	// FileNameKey gives the file's name in the collection.
	FileNameKey = "name"
)
