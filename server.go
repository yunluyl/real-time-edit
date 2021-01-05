package main

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

const (
	authHeader = "Sec-WebSocket-Protocol"
)

var hubs = make(map[string]*Hub)

func main() {
	defer firestoreClient.Close()
	router := mux.NewRouter()
	router.HandleFunc("/", wsHandler)
	//router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/out/")))
	addr := ":80"
	log.Println("Starting server at: http://" + addr)
	log.Fatal(http.ListenAndServe(addr, router))
}

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
	hub, ok := hubs[hubName]
	if !ok {
		var err error
		hub, err = createOrRetrieveHub(hubName, userID)
		if err != nil {
			log.Printf("Error creating hub %s: %v", hubName, err)
			return
		}
		hubs[hubName] = hub
		go hub.run()
	}
	serveWs(userID, hub, w, r)
}

const optionalPrefix = "Bearer|"

// userIDFromHeader checks the protocol header of the Websocket connection and decodes
// it if it exists.
func userIDFromHeader(header string) string {
	token := strings.TrimPrefix(header, optionalPrefix)
	user, err := firebaseAuth.VerifyIDToken(context.Background(), token)
	if err != nil {
		log.Printf("Could not verify token %s:  %+v", token, err)
		return ""
	}
	return user.UID
}
