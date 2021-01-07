package main

import (
	"log"
	"net/http"
	"strings"

	"collabserver/hub"
	"collabserver/storage"

	"github.com/gorilla/mux"
)

const (
	authHeader = "Sec-WebSocket-Protocol"
)

var hubs = make(map[string]*hub.Hub)

func main() {
	defer storage.Close()
	router := mux.NewRouter()
	router.HandleFunc("/", wsHandler)
	//router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/out/")))
	addr := ":80"
	log.Println("Starting server at: http://" + addr)
	log.Fatal(http.ListenAndServe(addr, router))
}

// wsHandler handles incoming Websocket connections. It requires a hub in the url (i.e.
// localhost:8080?hub=ABCDEF) and an auth token in the Sec-WebSocket-Protocol header.
// In javascript/typescript, this is fulfilled by the second parameter of the Websocket
// constructor: (i.e. new WebSocket(url, header)).
func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Check for user ID.
	userID := userIDFromHeader(r.Header.Get(authHeader))
	if userID == "" {
		userID = "test"
	}
	log.Printf("Connecting user with idToken %s", userID)
	keys, ok := r.URL.Query()["hub"]

	if !ok || len(keys[0]) < 1 {
		log.Println("Url Param 'hub' is missing")
		return
	}
	hubName := keys[0]
	currentHub, ok := hubs[hubName]
	if !ok {
		var err error
		currentHub, err = hub.CreateOrRetrieveHub(hubName, userID)
		if err != nil {
			log.Printf("Error creating hub %s: %v", hubName, err)
			return
		}
		hubs[hubName] = currentHub
		go currentHub.Run()
	}
	protocol := r.Header.Get(authHeader)
	// Generate the "response", which is just the same header that was given in the request.
	response := http.Header{}
	response.Add(authHeader, protocol)
	hub.ServeWs(userID, currentHub, w, r, response)
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
