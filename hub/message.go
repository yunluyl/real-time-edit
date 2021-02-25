package hub

import "collabserver/collections"

const (
	endpointPassthrough       = "PASSTHROUGH"
	endpointFileUpdate        = "FILE_UPDATE"
	endpointFileCreate        = "FILE_CREATE"
	endpointFileRename        = "FILE_RENAME"
	endpointFileDelete        = "FILE_DELETE"
	endpointModifyUser        = "MODIFY_USER"
	endpointListUsers         = "LIST_USERS"
	endpointListFiles         = "LIST_FILES"
	endpointListHub           = "LIST_HUB"
	endpointConnectToHub      = "CONNECT_HUB"
	endpointDisconnectFromHub = "DISCONNECT_HUB"
	endpointHubCreate         = "HUB_CREATE"
	endpointFileRetrieve      = "FILE_RETRIEVE"

	routeBroadcast = "BROADCAST"
	routeOrigin    = "ORIGIN"

	userAdd    = "ADD"
	userRemove = "REMOVE"
	userModify = "MODIFY"
)

// Message defines the Websocket message between browser and this real-time server
type Message struct {
	// UID is used for file operations; clients won't send another message until their
	// outstanding message of the same id is sent back with a success.
	UID string `json:"uid"`
	// Endpoint specifies how the message should be handled, i.e. pushing file operations, connecting to a hub, etc.
	Endpoint string `json:"endpoint"`
	// Route is single item list of routeBroadcast or routeOrigin, or otherwise a list of clients to send the message to.
	Route []string `json:"route"`
	// Status provides information about the state of the request, such as a success or failure.
	Status string `json:"status"`
	// Text is intended to provide additional info about the Status if possible.
	Text string `json:"text"`
	// File is the name of the notebook file in the hub, e.g. Untitled.ipynb
	File string `json:"file"`
	// Index is the starting index of the operation if this is a file update request.
	Index int64 `json:"index"`
	// Operations is a list of OT operations done on the notebook as JSON-able strings.
	Operations []string `json:"operations"`

	// User modification fields
	// ModifyUserType is classification of the user change, e.g. add user, remove user, change role
	ModifyUserType string `json:"modifyUserType"`
	// ModifyUserRole is the role the user is being changed to if applicable
	ModifyUserRole string `json:"modifyUserRole"`
	// ModifyUserID is the email of the user being modified.
	ModifyUserID string `json:"modifyUserID"`

	// NewFileName is used when changing file names, in conjunction with File to indicate the new and old file names.
	NewFileName string `json:"newFileName"`
	// FileState is passed to the client on initial connection to a file to use as a base for applying operations.
	// It is also passed from the client to the backend on initial file load in case the backend needs it.
	FileState string `json:"fileState"`

	// UserList is passed to the client and lists members of the hub and their statuses.
	UserList []collections.UserInfo `json:"userList"`
	// FileList is the list of files associated with the hub.
	FileList []collections.FileInfo `json:"fileList"`
	// HubList is the hub codes associated with the users.
	HubList []string `json:"hubList"`

	HubName string `json:"hubName"`
	client  *Client
}
