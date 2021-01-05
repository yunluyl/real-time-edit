package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strconv"

	"collabserver/collabauth"

	"google.golang.org/api/iterator"

	"cloud.google.com/go/firestore"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
)

const (
	firestoreClientName     = "yunlu-test"
	operationCollectionName = "operations"
	authCollectionName      = "authorization"
)

var (
	db *collabStorage

	firebaseApp         *firebase.App
	firebaseAuth        *auth.Client
	firestoreClient     *firestore.Client
	operationCollection *firestore.CollectionRef
	authCollection      *firestore.CollectionRef

	errUnauthorized     = errors.New("User not authorized to perform action")
	errHubAlreadyExists = errors.New("Hub has already been created")
)

// OperationEntry is the schema of the database entry for transformational operations
type OperationEntry struct {
	Hub string `firestore:"hub"`

	File string `firestore:"file"`

	Index int64 `firestore:"index"`

	Op string `firestore:"op"`

	UserID string `firestore:"uid"`
}

func init() {
	db = &collabStorage{}
	db.init()
}

func generateDocID(operationEntry *OperationEntry) string {
	return base64.StdEncoding.EncodeToString([]byte(
		operationEntry.Hub +
			"###" +
			operationEntry.File +
			"###" +
			strconv.FormatInt(operationEntry.Index, 10)))
}

func create(operationEntry *OperationEntry) error {
	docRef := operationCollection.Doc(generateDocID(operationEntry))
	_, err := docRef.Create(context.Background(), operationEntry)
	if err != nil {
		return err
	}
	return nil
}

type collabStorage struct {
	app        *firebase.App
	client     *firestore.Client
	operations *firestore.CollectionRef
	auth       collabauth.Authenticator
	authRef    *firestore.CollectionRef
}

func (cs *collabStorage) init() {
	var err error
	// TODO: turn nil into a config.Config object
	cs.app, err = firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatalf("initiate Firebase App failed: %+v", err)
	}
	cs.client, err = firestore.NewClient(context.Background(), firestoreClientName)
	if err != nil {
		log.Fatalf("initiate Firestore client failed: %+v", err)
	}
	cs.operations = firestoreClient.Collection(operationCollectionName)
	if cs.operations == nil {
		log.Fatal("get operations collection failed")
	}
	cs.authRef = firestoreClient.Collection(authCollectionName)
	if cs.authRef == nil {
		log.Fatal("get authorization collection failed")
	}
	cs.auth = collabauth.CurrentAuthenticator(cs.authRef)
}

func (cs *collabStorage) commit(hubName,
	file string,
	idx int64,
	userID string,
	ops []string) (string, int64, []string, string) {
	// check userID if it can do this commit
	if ok, _ := cs.auth.CanCommit(userID, hubName); !ok {
		return statusEndpointUnauthorized, 0, nil, ""
	}

	retOps, start, err := cs.opsForFile(hubName, file, idx, ops)
	if err != nil {
		return statusOperationCommitError, idx, []string{}, err.Error()
	}
	if idx < 0 {
		if start == -1 {
			start = 0
		}
		return statusOperationTooOld, start, retOps, ""
	} else if start == idx-1 && (len(retOps) == 1 || idx == 0) {
		if len(ops) > 500 {
			msg := fmt.Sprintf("length of operations: %d in message is larger than 500", len(ops))
			log.Println(msg)
			return statusOperationCommitError, idx, []string{}, msg
		}
		batch := firestoreClient.Batch()
		for i, op := range ops {
			operationEntry := &OperationEntry{
				Hub:   hubName,
				File:  file,
				Index: idx + int64(i),
				Op:    op,
			}
			docRef := operationCollection.Doc(generateDocID(operationEntry))
			batch.Create(docRef, *operationEntry)
		}
		_, err := batch.Commit(context.Background())
		if err != nil {
			return statusOperationCommitError, idx, []string{}, err.Error()
		}
		return statusOperationCommitted, idx, ops, ""
	} else if start == -1 {
		log.Printf("operation index: %d larger than upper bound", idx)
		return statusOperationTooNew, idx, []string{}, ""
	} else if start > idx {
		log.Printf("operation index: %d smaller than lower bound: %d", idx, start)
		return statusOperationTooOld, start, retOps, ""
	} else {
		return statusOperationTooOld, idx, retOps[idx-start:], ""
	}
}

func (cs *collabStorage) opsForFile(hubName,
	file string,
	idx int64,
	ops []string) ([]string, int64, error) {
	iter := operationCollection.
		Where("hub", "==", hubName).
		Where("file", "==", file).
		Where("index", ">=", idx-1).
		OrderBy("index", firestore.Asc).
		Documents(context.Background())
	var start int64 = -1
	var count int64 = -1
	retOps := []string{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Query operation error: %+v", err)
			return []string{}, 0, err
		}
		data := &OperationEntry{}
		convertErr := doc.DataTo(data)
		if convertErr != nil {
			log.Printf("Data conversion error: %+v", err)
			return []string{}, 0, err
		}
		if start == -1 {
			start = data.Index
			count = data.Index
		} else if data.Index-count == 1 {
			count++
		} else {
			err = fmt.Errorf("Query operation not in sequence prev index: %d, cur index: %d", count, data.Index)
			log.Println(err)
			return []string{}, 0, err
		}
		retOps = append(retOps, data.Op)
	}
	return retOps, start, nil
}

// TODO(itsazhuhere@): implement these functions.
func (cs *collabStorage) createDoc(userID string, hub string) {
	if ok, _ := cs.auth.CanCreateDoc(userID, hub); !ok {
		return
	}
}

func (cs *collabStorage) deleteDoc(userID string, hub string) {
	if ok, _ := cs.auth.CanDeleteDoc(userID, hub); !ok {
		return
	}
}

func (cs *collabStorage) readDoc(userID string, hub string) {
	if ok, _ := cs.auth.CanRead(userID, hub); !ok {
		return
	}
}

func (cs *collabStorage) addUser(requester string, toAdd string, hub string, role string) error {
	// Changing a user's role is the same as adding them with a different role.
	// Removing a user from a hub is the same as changing their role to NO_ROLE for that hub.
	ok, docRef := cs.auth.CanChangeUsers(requester, hub)
	if !ok {
		log.Print("User can't change other users")
		return errUnauthorized
	}
	if docRef == nil {
		// Usually means the user's role hasn't been set for a hub, so we add them to it.
		var err error
		docRef, err = cs.auth.CreateEntry(toAdd, hub)
		if err != nil {
			return err
		}
	}
	update := firestore.Update{
		Path:  collabauth.Role,
		Value: role,
	}
	docRef.Update(context.Background(), []firestore.Update{update})
	return nil
}

// connectUser marks a user as actively viewing a hub (so that other users in the hub can see).
// This is different from adding a user to a hub, which
// For now it just checks that a user is able to view a hub.
// TODO(itsazhuhere@): implement user connection functionality.
func (cs *collabStorage) connectUser(userID string, hub string) bool {
	ok, _ := cs.auth.CanRead(userID, hub)
	return ok
}

func (cs *collabStorage) IDFromToken(token string) string {
	token, err := collabauth.UserIDFromToken(context.Background(), cs.app, token)
	if err != nil {
		fmt.Printf("Error getting user id: %v", err)
		return ""
	}
	return token
}

func (cs *collabStorage) hubOwners(hub string) []string {
	return cs.auth.HubOwners(hub)
}

func (cs *collabStorage) createHub(userID string, hub string) error {
	if len(cs.auth.HubOwners(hub)) > 0 {
		return errHubAlreadyExists
	}

	err := cs.addUser(userID, userID, hub, collabauth.Owner)
	return err
}
