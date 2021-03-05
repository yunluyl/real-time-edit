// Package remotejob encapsulates sending messages to remote services such as Pub/Sub.
package remotejob

import (
	"context"
	"encoding/json"

	log "collabserver/cloudlog"

	"cloud.google.com/go/pubsub"
)

const (
	projectID       = "yunlu-test"
	fileUpdateTopic = "realtime_collab_file_update"
	fileDeleteTopic = ""

	maxRequests = 100 // arbitrary number
)

var (
	client   *pubsub.Client
	requests *requestPool
)

// UpdateMessage holds the fields for a file update request through Pub/Sub.
type UpdateMessage struct {
	Hub  string `json:"hub"`
	File string `json:"file"`
}

func init() {
	var err error
	client, err = pubsub.NewClient(context.Background(), projectID)
	if err != nil {
		log.Printf("Failed to start pubsub client: %s", err.Error())
		return
	}
	requests = &requestPool{}
}

// FileUpdateRequest sends a file update request for the given hub and file name through Pub/Sub
func FileUpdateRequest(hub, fileName string) {
	log.Print("initiating file update")
	if client == nil {
		log.Print("client is nil")
		return
	}

	originalMessage := UpdateMessage{
		Hub:  hub,
		File: fileName,
	}
	log.Printf("sending message %#v", originalMessage)

	marshalledMessaged, err := json.Marshal(originalMessage)
	if err != nil {
		log.Printf("Error marshalling message %#v, reason: %s", originalMessage, err.Error())
	}
	log.Printf("marshalled message %#v", marshalledMessaged)

	topic := client.Topic(fileUpdateTopic)
	result := topic.Publish(context.Background(), &pubsub.Message{
		Data: marshalledMessaged,
	})

	requests.add(result)
}

// TODO (itsazhuhere@): implement a pool to ensure messages are being successfully sent/
// make sure not too many are being sent at once.
type requestPool struct {
}

func (rp *requestPool) add(result *pubsub.PublishResult) {
	// do nothing for now.
}
