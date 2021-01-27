package websocketcodes

const (
	// StatusOperationCommitted is given when the commit to db is successful.
	StatusOperationCommitted = "OP_COMMITTED"

	// StatusOperationCommitError is given when the commit to db fails.
	StatusOperationCommitError = "OP_COMMIT_ERR"

	// StatusOperationTooNew is given when the operation's index is greater than the current index.
	StatusOperationTooNew = "OP_TOO_NEW"

	// StatusOperationTooOld is given when the operation's index is less than the current index.
	StatusOperationTooOld = "OP_TOO_OLD"

	// StatusEndpointNotValid is given when the message is using an unsupported endpoint.
	StatusEndpointNotValid = "ENDPOINT_NOT_VALID"

	// StatusEndpointUnauthorized is given when the user is not authorized to perform an action.
	StatusEndpointUnauthorized = "ENDPOINT_UNAUTHORIZED"

	// StatusFileDoesntExist is given when the user tries to access a file that the hub does not have.
	StatusFileDoesntExist = "FILE_DOESNT_EXIST"

	// StatusFileExists is given when a user tries to create a file that already exists in the hub
	// (most likely a duplicate file name issue).
	StatusFileExists = "FILE_ALREADY_EXISTS"

	// StatusFileCreateFailed is given when the file creation failed for some reason.
	StatusFileCreateFailed = "CREATE_FILE_FAILED"
)
