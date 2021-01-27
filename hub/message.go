package hub

import "collabserver/collections"

const (
	endpointPassthrough = "PASSTHROUGH"
	endpointFileUpdate  = "FILE_UPDATE"
	endpointFileCreate  = "FILE_CREATE"
	endpointModifyUser  = "MODIFY_USER"
	endpointListUsers   = "LIST_USERS"
	endpointListFiles   = "LIST_FILES"

	routeBroadcast = "BROADCAST"
	routeOrigin    = "ORIGIN"

	userAdd    = "ADD"
	userRemove = "REMOVE"
	userModify = "MODIFY"
)

// Message defines the Websocket message between browser and this real-time server
type Message struct {
	UID        string   `json:"uid"`
	Endpoint   string   `json:"endpoint"`
	Route      []string `json:"route"`
	Status     string   `json:"status"`
	Text       string   `json:"text"`
	File       string   `json:"file"`
	Index      int64    `json:"index"`
	Operations []string `json:"operations"`

	// The request type, i.e.
	ModifyUserType string `json:"modifyUserType"`
	ModifyUserRole string `json:"modifyUserRole"`
	ModifyUserID   string `json:"modifyUserID"`

	UserList []collections.UserInfo `json:"userList"`
	FileList []collections.FileInfo `json:"fileList"`

	hubName string
	client  *Client
}
