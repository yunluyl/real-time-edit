package main

import (
	"net/http"
	"strings"

	log "collabserver/cloudlog"
	"collabserver/hub"
	"collabserver/storage"

	"github.com/gorilla/mux"
)

const (
	authHeader = "Sec-WebSocket-Protocol"
)

var hubConnector *hub.Connector

func main() {
	defer storage.Close()
	router := mux.NewRouter()
	router.HandleFunc("/", wsHandler)
	//router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/out/")))

	hubConnector = hub.NewConnector()

	addr := ":80"
	log.Println("Starting server at: http://" + addr)
	log.Fatal(http.ListenAndServe(addr, router))
}

// wsHandler handles incoming Websocket connections.
// It requires an auth token in the Sec-WebSocket-Protocol header.
// In javascript/typescript, this is fulfilled by the second parameter of the Websocket
// constructor: (i.e. new WebSocket(url, header)).
func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Check for user ID.
	userID := userIDFromHeader(r.Header.Get(authHeader))
	if userID == "" {
		log.Println("User token not provided")
		return
	}
	log.Printf("Connecting user with idToken %s", userID)
	protocol := r.Header.Get(authHeader)
	// Generate the "response", which is just the same header that was given in the request.
	response := http.Header{}
	response.Add(authHeader, protocol)
	hubConnector.ServeWs(userID, w, r, response)
}

const optionalPrefix = "Bearer|"

// userIDFromHeader checks the protocol header of the Websocket connection and decodes
// it if it exists.
func userIDFromHeader(header string) string {
	token := strings.TrimPrefix(header, optionalPrefix)
	userID, err := storage.DB.VerifyIDToken(token)
	if err != nil {
		log.Printf("Could not verify token %s:  %+v", token, err)
		return ""
	}
	return userID
}
