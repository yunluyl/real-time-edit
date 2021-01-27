package hub

import (
	"collabserver/collabauth"
	"collabserver/hubcodes"
	wscodes "collabserver/websocketcodes"
	"context"
	"log"

	"cloud.google.com/go/firestore"
)

func (h *Hub) processMessage(message *Message) *Message {
	switch message.Endpoint {
	case endpointPassthrough:
		return message
	case endpointFileUpdate:
		return h.handleFileUpdate(message)
	case endpointFileCreate:
		msg := h.handleFileCreate(message)
		log.Printf("return message for file create: %#v", msg)
		return msg
	case endpointModifyUser:
		return h.handleModifyUser(message)
	default:
		log.Printf("Message endpoint: " + message.Endpoint + " is not supported")
		ret := &Message{
			UID:      message.UID,
			Endpoint: message.Endpoint,
			Route:    append([]string{}, routeOrigin),
			Status:   wscodes.StatusEndpointNotValid,
		}
		return ret
	}
}

func (h *Hub) handleFileUpdate(message *Message) *Message {
	ret := &Message{
		UID:      message.UID,
		Endpoint: message.Endpoint,
		File:     message.File,
		hubName:  message.hubName,
	}
	var status string
	var idx int64
	var retOps []string
	var text string

	// Check if the user can update files.
	if ok, _ := h.auth.CanCommit(message.client.userID); !ok {
		status = wscodes.StatusEndpointUnauthorized
	} else {
		// Next find the document reference for the file.
		file, err := h.refForFilename(message.File)
		if err != nil {
			log.Printf("error from refForFilename: %s", err.Error())
			status = wscodes.StatusFileDoesntExist
			text = err.Error()
		} else {
			// Commit the operations since the previous two checks succeeded.
			status, idx, retOps, text = h.db.CommitOps(
				file,
				message.Index,
				message.Operations,
				message.client.userID)
		}
	}

	ret.Status = status
	ret.Index = idx
	ret.Operations = retOps
	ret.Text = text
	if status == wscodes.StatusOperationCommitted {
		ret.Route = append(ret.Route, routeBroadcast)
	} else {
		ret.Route = append(ret.Route, routeOrigin)
	}
	return ret
}

func (h *Hub) toOriginWithStatus(message *Message, status string, text string) *Message {
	return &Message{
		UID:      message.UID,
		Status:   status,
		Text:     text,
		Endpoint: message.Endpoint,
		Route:    append([]string{}, routeOrigin),
	}
}

func (h *Hub) refForFilename(fileName string) (*firestore.CollectionRef, error) {
	_, fileRef, err := h.db.DocExists(fileName, h.files)
	return h.db.CollectionForID(opsID, fileRef), err
}

type fileEntry struct {
	name string `firestore:"name"`
}

func (h *Hub) handleFileCreate(message *Message) *Message {
	log.Printf("creating file: %s", message.File)
	// Check if the file exists first, and if it does then return an error so we don't overwrite it.
	exists, fileRef, err := h.db.DocExists(message.File, h.files)
	if err != nil {
		return h.toOriginWithStatus(message, err.Error(), err.Error())
	}
	if exists {
		return h.toOriginWithStatus(message, wscodes.StatusFileExists, "")
	}
	// We can reuse the fileRef here because it represents the correct doc ID without needing to explicitly
	// exist in the collection.
	_, err = fileRef.Create(context.Background(), fileEntry{
		name: message.File,
	})
	if err != nil {
		return h.toOriginWithStatus(message, wscodes.StatusFileCreateFailed, "")
	}
	returnMessage := h.toOriginWithStatus(message, wscodes.StatusOperationCommitted, "")
	returnMessage.File = message.File
	returnMessage.hubName = message.hubName

	return returnMessage
}

func (h *Hub) handleModifyUser(message *Message) *Message {
	var err error
	switch message.ModifyUserType {
	case userAdd:
		fallthrough
	case userModify:
		err = h.AddUser(message.ModifyUserID, message.client.userID, message.ModifyUserRole)
	case userRemove:
		err = h.AddUser(message.ModifyUserID, message.client.userID, collabauth.NoRole)
	}
	ret := &Message{
		UID:      message.UID,
		Endpoint: message.Endpoint,
		File:     message.File,
		hubName:  message.hubName,
	}

	if err != nil {
		ret.Route = append(ret.Route, routeOrigin)
		ret.Status = wscodes.StatusEndpointUnauthorized
	} else {
		ret.Route = append(ret.Route, routeBroadcast)
		ret.Status = wscodes.StatusOperationCommitted
	}
	return ret
}

// AddUser adds a user, after first checking if requester is able to add users.
// Since a hub requires an owner on init, calling this function should mean at least one owner exists.
func (h *Hub) AddUser(toAdd, requester, role string) error {
	ok, docRef := h.auth.CanChangeUsers(requester)
	if !ok {
		log.Print("User can't change other users")
		return errUnauthorized
	}
	if docRef == nil {
		// Usually means the user's role hasn't been set for a hub, so we add them to it.
		var err error
		docRef, err = h.db.AddEntry(h.users, toAdd, collabauth.AuthEntry{
			UserID: toAdd,
			Role:   collabauth.NoRole,
			Status: hubcodes.UserOffline,
		})
		if err != nil {
			return err
		}
	}
	update := firestore.Update{
		Path:  collabauth.Role,
		Value: role,
	}
	_, err := docRef.Update(context.Background(), []firestore.Update{update})
	return err
}

// ConnectUser marks a user as actively viewing a hub (so that other users in the hub can see).
// For now it just checks that a user is able to view a hub.
func (h *Hub) ConnectUser(userID string) error {
	ok, docRef := h.auth.CanRead(userID)
	if !ok {
		return errUnauthorized
	}
	err := h.db.UpdateEntry(docRef, hubcodes.UserStatusKey, hubcodes.UserOnline)
	return err
}

// DisconnectUser marks a user as Offline.
func (h *Hub) DisconnectUser(userID string) error {
	exists, docRef, err := h.db.DocExists(userID, h.users)
	if err != nil {
		return err
	}
	if !exists {
		return ErrorEntryNotFound
	}
	err = h.db.UpdateEntry(docRef, hubcodes.UserStatusKey, hubcodes.UserOnline)
	return err
}
