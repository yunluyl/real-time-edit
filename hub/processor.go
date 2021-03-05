package hub

import (
	log "collabserver/cloudlog"
	"collabserver/collabauth"
	"collabserver/collections"
	"collabserver/hubcodes"
	"collabserver/remotejob"
	wscodes "collabserver/websocketcodes"
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func (h *Hub) processMessage(message *Message) *Message {
	switch message.Endpoint {
	case endpointPassthrough:
		return message
	case endpointFileRetrieve:
		return h.handleFileRetrieve(message)
	case endpointFileUpdate:
		return h.handleFileUpdate(message)
	case endpointFileCreate:
		return h.handleFileCreate(message)
	case endpointFileRename:
		return h.handleFileRename(message)
	case endpointFileDelete:
		return h.handleFileDelete(message)
	case endpointListUsers:
		return h.handleListUser(message)
	case endpointModifyUser:
		return h.handleModifyUser(message)
	case endpointListFiles:
		return h.handleListFiles(message)
	case endpointDisconnectFromHub:
		go h.handBackClient(message.client)
		return nil
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

func (h *Hub) handleFileRetrieve(message *Message) *Message {
	if ok, _ := h.auth.CanCommit(message.client.userID); !ok {
		return toOriginWithStatus(message, wscodes.StatusEndpointUnauthorized, "")
	}
	data := collections.FileInfo{}
	docRef, err := h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, message.File, &data)
	if err != nil {
		log.Printf("error from refForFilename: %s", err.Error())
		return toOriginWithStatus(message, wscodes.StatusFileDoesntExist, err.Error())
	}
	// A bit of incrementing here because OpsFor File gives ops starting from idx-1, and
	// the snapshot's index is the index of the latest op it's updated to (i.e. if the latest op is
	// index 1, and snapshot is caught up then snapshot.Index == 1). Leaving it as is will cause it
	// to replay the last two ops on top of the current file state.
	idx := int64(data.Snapshot.Index) + 2
	ops, _, err := h.db.OpsForFile(h.db.CollectionForID(opsID, docRef), idx)
	if err != nil {
		return toOriginWithStatus(message, wscodes.StatusFailure, err.Error())
	}

	returnMessage := toOriginWithStatus(message, wscodes.StatusSuccess, "")

	if idx == -1 && len(ops) == 0 {
		// File is empty and needs an initial file state
		// commit message's filestate
		if message.FileState != "" {
			err := h.db.UpdateEntry(docRef, "snapshot", collections.FileSnapshot{File: message.FileState})
			if err != nil {
				log.Printf("Updating intial file state failed: %#v", err)
				returnMessage.FileState = data.Snapshot.File
			} else {
				returnMessage.FileState = message.FileState
			}
		} else {
			log.Printf("handleFileRetrieve attempted to set initial file state but message had empty FileState: %#v", message)
		}
	} else {
		returnMessage.FileState = data.Snapshot.File
	}

	returnMessage.Operations = ops
	returnMessage.Index = idx - 2
	returnMessage.File = message.File

	return returnMessage
}

func (h *Hub) handleFileUpdate(message *Message) *Message {
	ret := &Message{
		UID:      message.UID,
		Endpoint: message.Endpoint,
		File:     message.File,
		HubName:  message.HubName,
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
		data := collections.FileInfo{}
		docRef, err := h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, message.File, &data)
		if err != nil {
			log.Printf("error from refForFilename: %s", err.Error())
			status = wscodes.StatusFileDoesntExist
			text = err.Error()
		} else {
			file := h.db.CollectionForID(opsID, docRef)
			// Commit the operations since the previous two checks succeeded.
			status, idx, retOps, text = h.db.CommitOps(
				file,
				message.Index,
				message.Operations,
				message.client.userID,
			)
			
			// check the latest operation index against data.Index
			if !data.MarkedForUpdate && status == wscodes.StatusOperationCommitted {
				latestOpIndex := int(idx) + len(retOps)
				if latestOpIndex - data.Snapshot.Index > maxOpsBeforeUpdate {
					// mark it as needing an update; the remote service will read and perform
					// the necessary transaction atomically (i.e. if this is called multiple times and
					// one of the runs finishes then all other runs will terminate without any writes).
					h.db.UpdateEntry(docRef, hubcodes.FileUpdateKey, true)
					remotejob.FileUpdateRequest(h.name, message.File)
				}
			}

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

func (h *Hub) handleFileRename(message *Message) *Message {
	if ok, _ := h.auth.CanCommit(message.client.userID); !ok {
		return toOriginWithStatus(message, wscodes.StatusEndpointUnauthorized, "")
	}
	fileEntry := &collections.FileInfo{}

	// Check if the old file name actually exists.
	docRef, err := h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, message.File, fileEntry)
	if err != nil {
		if err == iterator.Done {
			return toOriginWithStatus(message, wscodes.StatusFileDoesntExist, "")
		}
		return toOriginWithStatus(message, err.Error(), err.Error())
	}

	// Check if the file we're changing to exists; we don't want it to already exist.
	_, err = h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, message.NewFileName, fileEntry)
	if err != iterator.Done {
		return toOriginWithStatus(message, wscodes.StatusFileExists, "")
	}

	// Attempt to rename, returning the error or a success depending on the result.
	err = h.db.UpdateEntry(docRef, hubcodes.FileNameKey, message.NewFileName)
	if err != nil {
		return toOriginWithStatus(message, wscodes.StatusFileCreateFailed, err.Error())
	}

	returnMessage := toOriginWithStatus(message, wscodes.StatusOperationCommitted, "")
	returnMessage.File = message.NewFileName

	return returnMessage
}

func (h *Hub) refForFilename(fileName string) (*firestore.CollectionRef, error) {
	fileRef, err := h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, fileName, &collections.FileInfo{})
	return h.db.CollectionForID(opsID, fileRef), err
}

func (h *Hub) handleFileCreate(message *Message) *Message {
	if ok, _ := h.auth.CanCommit(message.client.userID); !ok {
		return toOriginWithStatus(message, wscodes.StatusEndpointUnauthorized, "")
	}

	// Check if the file exists first, and if it does then return an error so we don't overwrite it.
	fileEntry := &collections.FileInfo{}
	_, err := h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, message.File, fileEntry)
	if err != iterator.Done {
		if err != nil {
			return toOriginWithStatus(message, err.Error(), err.Error())
		}
		return toOriginWithStatus(message, wscodes.StatusFileExists, "")
	}
	// Proceed with creating the file.
	fileEntry = &collections.FileInfo{
		Name:     message.File,
		Snapshot: collections.FileSnapshot{Index: -1},
	}
	_, err = h.db.AddEntry(h.files, "", fileEntry)
	if err != nil {
		return toOriginWithStatus(message, wscodes.StatusFileCreateFailed, "")
	}
	// Return success message.
	returnMessage := toOriginWithStatus(message, wscodes.StatusOperationCommitted, "")
	returnMessage.File = message.File

	return returnMessage
}

func (h *Hub) handleFileDelete(message *Message) *Message {
	if ok, _ := h.auth.CanCommit(message.client.userID); !ok {
		return toOriginWithStatus(message, wscodes.StatusEndpointUnauthorized, "")
	}

	// Check if the file exists first before trying to delete
	fileEntry := &collections.FileInfo{}
	docRef, err := h.db.EntryForFieldValue(h.files, hubcodes.FileNameKey, message.File, fileEntry)
	if err != nil {
		if err == iterator.Done {
			return toOriginWithStatus(message, wscodes.StatusFileDoesntExist, "")
		}
		return toOriginWithStatus(message, err.Error(), err.Error())
	}

	err = h.db.DeleteDocument(docRef)
	if err != nil {
		toOriginWithStatus(message, err.Error(), err.Error())
	}
	returnMessage := toOriginWithStatus(message, wscodes.StatusOperationCommitted, "")

	return returnMessage
}

func (h *Hub) handleModifyUser(message *Message) *Message {
	var err error
	switch message.ModifyUserType {
	case userAdd:
		// TODO(itsazhuhere@): Change this back to collabauth.Reader; temporarily changed to writer for demo.
		err = h.AddUser(message.ModifyUserID, message.client.userID, collabauth.Writer)
	case userModify:
		err = h.AddUser(message.ModifyUserID, message.client.userID, message.ModifyUserRole)
	case userRemove:
		err = h.AddUser(message.ModifyUserID, message.client.userID, collabauth.NoRole)
	}
	ret := &Message{
		UID:      message.UID,
		Endpoint: message.Endpoint,
		File:     message.File,
		HubName:  message.HubName,
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

func (h *Hub) handleListUser(message *Message) *Message {
	userList, err := h.listUsers(message.client.userID)
	if err != nil {
		return toOriginWithStatus(message, wscodes.StatusEndpointUnauthorized, "")
	}
	// TODO(itsazhuhere@): this should really be a different status, because it might be confusing.
	msg := toOriginWithStatus(message, wscodes.StatusOperationCommitted, "")
	msg.UserList = userList
	return msg
}

func (h *Hub) listUsers(requester string) ([]collections.UserInfo, error) {
	if ok, _ := h.auth.CanRead(requester); !ok {
		return nil, errUnauthorized
	}

	return h.allUsers()
}

func (h *Hub) allUsers() ([]collections.UserInfo, error) {
	return h.db.AllUsers(h.users)
}

func (h *Hub) handleListFiles(message *Message) *Message {
	fileList, err := h.listFiles(message.client.userID)
	if err != nil {
		return toOriginWithStatus(message, wscodes.StatusEndpointUnauthorized, "")
	}
	// TODO(itsazhuhere@): this should really be a different status, because it might be confusing.
	msg := toOriginWithStatus(message, wscodes.StatusOperationCommitted, "")
	msg.FileList = fileList
	return msg
}

func (h *Hub) listFiles(requester string) ([]collections.FileInfo, error) {
	if ok, _ := h.auth.CanRead(requester); !ok {
		return nil, errUnauthorized
	}
	return h.db.AllFiles(h.files)
}

// AddUser adds a user, after first checking if requester is able to add users.
// Since a hub requires an owner on init, calling this function should mean at least one owner exists.
func (h *Hub) AddUser(toAdd, requester, role string) error {
	log.Printf("Adding user %s as %s to hub %s requested by %s", toAdd, role, h.name, requester)
	ok, _ := h.auth.CanChangeUsers(requester)
	if !ok {
		log.Print("User can't change other users")
		return errUnauthorized
	}
	userIDs, err := h.db.UserIDsForEmails([]string{toAdd})
	if err != nil {
		return err
	}
	var userID string
	if userID, ok = userIDs[toAdd]; !ok {
		return errors.New("Email not found")
	}
	log.Printf("Got id from email: %s", userID)
	authEntry := &collections.AuthEntry{}
	// Currenly just using this for checking for the existence of docRef.
	docRef, _ := h.db.EntryForFieldValue(h.users, hubcodes.UserIDKey, userID, authEntry)
	if docRef == nil {
		// Usually means the user's role hasn't been set for a hub, so we add them to it.
		var err error
		docRef, err = h.db.AddEntry(h.users, "", collections.AuthEntry{
			UserID: userID,
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
	_, err = docRef.Update(context.Background(), []firestore.Update{update})
	if err != nil {
		return err
	}
	// Also update their entry in the user to hubs collection
	h.db.UpdateUsersHubList(userID, h.name, role)

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

func (h *Hub) hubConnectSuccessMessage(client *Client) *Message {
	return &Message{
		Status:   wscodes.StatusSuccess,
		Endpoint: endpointConnectToHub,
		Route:    append([]string{}, routeOrigin),
		client:   client,
		HubName:  h.name,
	}
}

func toOriginWithStatus(message *Message, status string, text string) *Message {
	return &Message{
		UID:      message.UID,
		Status:   status,
		Text:     text,
		Endpoint: message.Endpoint,
		Route:    append([]string{}, routeOrigin),
	}
}
