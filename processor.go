package main

import (
	"collabserver/collabauth"
	"log"
)

func processMessage(message *Message) *Message {
	switch message.Endpoint {
	case endpointPassthrough:
		return message
	case endpointFileUpdate:
		return handleFileUpdate(message)
	case endpointModifyUser:
		return handleModifyUser(message)
	default:
		log.Printf("Message endpoint: " + message.Endpoint + " is not supported")
		ret := &Message{
			UID:      message.UID,
			Endpoint: message.Endpoint,
			Route:    append([]string{}, routeOrigin),
			Status:   statusEndpointNotValid,
		}
		return ret
	}
}

func handleFileUpdate(message *Message) *Message {
	ret := &Message{
		UID:      message.UID,
		Endpoint: message.Endpoint,
		File:     message.File,
		hubName:  message.hubName,
	}
	status, idx, retOps, text := db.commit(
		message.hubName,
		message.File,
		message.Index,
		message.client.userID,
		message.Operations)
	ret.Status = status
	ret.Index = idx
	ret.Operations = retOps
	ret.Text = text
	if status == statusOperationCommitted {
		ret.Route = append(ret.Route, routeBroadcast)
	} else {
		ret.Route = append(ret.Route, routeOrigin)
	}
	return ret
}

func handleModifyUser(message *Message) *Message {
	var err error
	switch message.ModifyUserType {
	case userAdd:
		fallthrough
	case userModify:
		err = db.addUser(message.client.userID, message.ModifyUserID, message.hubName, message.ModifyUserRole)
	case userRemove:
		err = db.addUser(message.client.userID, message.ModifyUserID, message.hubName, collabauth.NoRole)
	}
	ret := &Message{
		UID:      message.UID,
		Endpoint: message.Endpoint,
		File:     message.File,
		hubName:  message.hubName,
	}

	if err != nil {
		ret.Route = append(ret.Route, routeOrigin)
		ret.Status = statusEndpointUnauthorized
	} else {
		ret.Route = append(ret.Route, routeBroadcast)
		ret.Status = statusOperationCommitted
	}
	return ret
}
