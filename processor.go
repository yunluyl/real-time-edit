package main

import "log"

func processMessage(message *Message) *Message {
	switch message.Endpoint {
	case endpointPassthrough:
		return message
	case endpointFileUpdate:
		ret := &Message{
			UID:      message.UID,
			Endpoint: message.Endpoint,
			File:     message.File,
			hubName:  message.hubName,
		}
		status, idx, retOps, text := commitOperations(
			message.hubName,
			message.File,
			message.Index,
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
