package main

import (
	"log"
    "net/http"
	"strconv"

	"github.com/gorilla/mux"
)

var hubs = make(map[string]*Hub)
var userId = 0

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", wsHandler)
	//router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/out/")))
	addr := ":80"
	log.Println("Starting server at: http://" + addr)
	log.Fatal(http.ListenAndServe(addr, router))
}


func wsHandler(w http.ResponseWriter, r *http.Request) {
	// TODO(yunlu): authentication, get user ID, IAM check for hub access
	keys, ok := r.URL.Query()["hub"]

	if !ok || len(keys[0]) < 1 {
		log.Println("Url Param 'hub' is missing")
		return
	}
	hubName := keys[0]
	hub, ok := hubs[hubName]
	if !ok {
		hub = newHub(hubName)
		hubs[hubName] = hub
		go hub.run()
	}
	serveWs(strconv.Itoa(userId), hub, w, r)
	userId++
}
