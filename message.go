package main

const (
	endpointPassthrough = "PASSTHROUGH"
	endpointFileUpdate = "FILE_UPDATE"

	routeBroadcast = "BROADCAST"
	routeOrigin = "ORIGIN"

	statusOperationCommitted = "OP_COMMITTED"
	statusOperationTooNew = "OP_TOO_NEW"
	statusOperationTooOld = "OP_TOO_OLD"
	statusEndpointNotValid = "ENDPOINT_NOT_VALID"
)

type Message struct {
	UID string `json:"uid"`

	Endpoint string `json:"endpoint"`

	Route [] string `json:"route"`

	Status string `json:"status"`

	Text string `json:"text"`

    Index int64 `json:"index"`

	Operations [] string `json:"operations"`

	client *Client
}