package main

const (
	endpointPassthrough = "PASSTHROUGH"
	endpointFileUpdate  = "FILE_UPDATE"

	routeBroadcast = "BROADCAST"
	routeOrigin    = "ORIGIN"

	statusOperationCommitted   = "OP_COMMITTED"
	statusOperationCommitError = "OP_COMMIT_ERR"
	statusOperationTooNew      = "OP_TOO_NEW"
	statusOperationTooOld      = "OP_TOO_OLD"
	statusEndpointNotValid     = "ENDPOINT_NOT_VALID"
)

// Message defines the Websocket message between browser and this real-time server
type Message struct {
	UID string `json:"uid"`

	Endpoint string `json:"endpoint"`

	Route []string `json:"route"`

	Status string `json:"status"`

	Text string `json:"text"`

	File string `json:"file"`

	Index int64 `json:"index"`

	Operations []string `json:"operations"`

	hubName string

	client *Client
}
