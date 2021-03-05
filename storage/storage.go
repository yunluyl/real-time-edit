package storage

import (
	log "collabserver/cloudlog"
	"collabserver/collections"
	wscodes "collabserver/websocketcodes"
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	firestoreClientName     = "yunlu-test"
	operationCollectionName = "operations"
	authCollectionName      = "authorization"
	usersCollectionName     = "usersToHubs"

	// Indices are in base 10 (used for converting int to string)
	intBase = 10

	deletedField  = "deleted"
	hubUsersField = "users"
	userIDField   = "userID"
	roleField     = "role"
	hubPath       = "hub"
)

var (
	// DB represents a Firestore database, and contains functions for interacting with that database.
	DB *collabStorage
)

func init() {
	DB = &collabStorage{}
	DB.init()
}

// Close performs cleanup for closing storage connections.
func Close() {
	DB.client.Close()
}

// Storage defines the methods necessary for interacting with the underlying datastore.
// Useful for dependency injection in testing.
type Storage interface {
}

// OperationEntry is the schema of the database entry for transformational operations
type OperationEntry struct {
	Index int64 `firestore:"index"`

	Op string `firestore:"op"`

	UserID string `firestore:"userID"`
}

type collabStorage struct {
	app    *firebase.App
	auth   *auth.Client
	users  *firestore.CollectionRef
	client *firestore.Client
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
	cs.auth, err = cs.app.Auth(context.Background())
	if err != nil {
		log.Fatalf("initiate Firestore Auth failed: %+v", err)
	}

	cs.users = cs.client.Collection(usersCollectionName)
}

func (cs *collabStorage) CollectionForID(collectionID string, docRef *firestore.DocumentRef) *firestore.CollectionRef {
	if docRef == nil {
		return cs.client.Collection(collectionID)
	}
	return docRef.Collection(collectionID)
}

// DocExists checks for the existence of the document with ID docID within the given collection.
// If it exists, this function also returns a reference to the Document.
// It checks the error returned from docRef.Get and silences a codes.NotFound error because
// that info is reflected in the bool return.
func (cs *collabStorage) DocExists(docID string, collection *firestore.CollectionRef) (bool, *firestore.DocumentRef, error) {
	docRef := collection.Doc(docID)
	snapshot, err := docRef.Get(context.Background())
	if err != nil && status.Code(err) == codes.NotFound {
		err = nil
	}
	exists := snapshot != nil && snapshot.Exists()
	return exists, docRef, err
}

// EntryForFieldValue searches in the collection's fields for the provided value and if found puts the data
// into the provided struct pointer. It also returns the DocumentRef if it exists.
func (cs *collabStorage) EntryForFieldValue(collection *firestore.CollectionRef, fieldPath string, value, dataTo interface{}) (*firestore.DocumentRef, error) {
	iter := collection.
		Where(fieldPath, "==", value).
		Documents(context.Background())

	// If the "deleted" field exists and is true, move on to the next one.
	var doc *firestore.DocumentSnapshot
	for {
		var err error
		doc, err = iter.Next()
		if err != nil {
			return nil, err
		}
		fields := map[string]interface{}{}
		err = doc.DataTo(&fields)
		if err != nil {
			continue
		}
		if deleted, ok := fields[deletedField]; ok {
			if deleted.(bool) {
				continue
			}
			// At this point all scenarios result in us returning the current doc.
		}
		break
	}
	var err error
	if dataTo != nil {
		err = doc.DataTo(dataTo)
	}
	return doc.Ref, err
}

func (cs *collabStorage) CollectionIsEmpty(collection *firestore.CollectionRef) bool {
	allDocs, _ := collection.Documents(context.Background()).GetAll()
	return len(allDocs) == 0
}

func (cs *collabStorage) TopLevelCollection(collection string) *firestore.CollectionRef {
	return cs.client.Collection(collection)
}

// CommitOps checks that the OT operations can be committed then pushes them to the collection.
func (cs *collabStorage) CommitOps(opsCollection *firestore.CollectionRef, idx int64, ops []string, committerID string) (string, int64, []string, string) {
	retOps, start, err := cs.OpsForFile(opsCollection, idx)
	if err != nil {
		return wscodes.StatusOperationCommitError, idx, []string{}, err.Error()
	}
	if idx < 0 {
		if start == -1 {
			start = 0
		}
		return wscodes.StatusOperationTooOld, start, retOps, ""
	} else if start == idx-1 && (len(retOps) == 1 || idx == 0) {
		if len(ops) > 500 {
			msg := fmt.Sprintf("length of operations: %d in message is larger than 500", len(ops))
			log.Println(msg)
			return wscodes.StatusOperationCommitError, idx, []string{}, msg
		}
		batch := cs.client.Batch()
		for i, op := range ops {
			index := idx + int64(i)
			operationEntry := &OperationEntry{
				Index:  index,
				Op:     op,
				UserID: committerID,
			}
			// Generates a Doc with a random ID; we already access indices by Where queries so
			// there's no need to have a predictable ID (and reads are faster when they're random).
			docRef := opsCollection.NewDoc()
			batch.Create(docRef, *operationEntry)
		}
		_, err := batch.Commit(context.Background())
		if err != nil {
			return wscodes.StatusOperationCommitError, idx, []string{}, err.Error()
		}
		return wscodes.StatusOperationCommitted, idx, ops, ""
	} else if start == -1 {
		log.Printf("operation index: %d larger than upper bound", idx)
		return wscodes.StatusOperationTooNew, idx, []string{}, ""
	} else if start > idx {
		log.Printf("operation index: %d smaller than lower bound: %d", idx, start)
		return wscodes.StatusOperationTooOld, start, retOps, ""
	} else {
		return wscodes.StatusOperationTooOld, idx, retOps[idx-start:], ""
	}
}

// OpsForFile gives the operations starting from index idx.
func (cs *collabStorage) OpsForFile(opsCollection *firestore.CollectionRef, idx int64) ([]string, int64, error) {
	iter := opsCollection.
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

func (cs *collabStorage) AddEntry(collection *firestore.CollectionRef, id string, data interface{}) (*firestore.DocumentRef, error) {
	var docRef *firestore.DocumentRef
	if id == "" {
		docRef = collection.NewDoc()
	} else {
		docRef = collection.Doc(id)
	}

	_, err := docRef.Create(context.Background(), data)
	return docRef, err
}

func (cs *collabStorage) UpdateEntry(docRef *firestore.DocumentRef, path string, value interface{}) error {
	update := firestore.Update{
		Path:  path,
		Value: value,
	}
	_, err := docRef.Update(context.Background(), []firestore.Update{update})
	return err
}

// DeleteDocument marks the document as deleted by adding a field to it indicating so.
func (cs *collabStorage) DeleteDocument(docRef *firestore.DocumentRef) error {
	return cs.UpdateEntry(docRef, deletedField, true)
}

func (cs *collabStorage) VerifyIDToken(idToken string) (string, error) {
	token, err := cs.auth.VerifyIDToken(context.Background(), idToken)
	if err != nil {
		return "", err
	}
	return token.UID, nil
}

func (cs *collabStorage) allDocs(collection *firestore.CollectionRef) ([]*firestore.DocumentSnapshot, error) {
	docs, err := collection.Documents(context.Background()).GetAll()
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func (cs *collabStorage) AllUsers(collection *firestore.CollectionRef) ([]collections.UserInfo, error) {
	docs, err := cs.allDocs(collection)
	if err != nil {
		return nil, err
	}
	// Map of user IDs to user info in the hub
	idToInfo := map[string]collections.UserInfo{}
	userIDs := []string{}
	// Iterate over docs and get entries, with some bookkeeping to later get user emails and combine everything
	// to return to the client
	for _, doc := range docs {
		roleEntry := &collections.AuthEntry{}
		err := doc.DataTo(roleEntry)
		if err != nil {
			log.Printf("Error while getting user ID: %s", err.Error())
			continue
		}
		id := roleEntry.UserID
		idToInfo[id] = collections.UserInfo{
			Role:   roleEntry.Role,
			Status: roleEntry.Status,
		}
		userIDs = append(userIDs, id)
	}
	emails, err := cs.UserEmails(userIDs)
	if err != nil {
		return nil, err
	}
	userInfos := []collections.UserInfo{}
	for userID, email := range emails {
		currEntry := idToInfo[userID]
		currEntry.Email = email
		userInfos = append(userInfos, currEntry)
	}

	return userInfos, nil
}

func (cs *collabStorage) UserEmails(userIDs []string) (map[string]string, error) {
	emails := map[string]string{}
	for _, id := range userIDs {
		userRecord, err := cs.auth.GetUser(context.Background(), id)
		if err != nil {
			log.Printf("error while getting user email: %s", err.Error())
			continue
		}

		emails[id] = userRecord.Email
	}

	return emails, nil
}

func (cs *collabStorage) UserIDsForEmails(emails []string) (map[string]string, error) {
	ids := map[string]string{}
	for _, email := range emails {
		userRecord, err := cs.auth.GetUserByEmail(context.Background(), email)
		if err != nil {
			log.Printf("error while getting user id with email (%s): %s", email, err.Error())
			continue
		}

		ids[email] = userRecord.UID
	}

	return ids, nil
}

func (cs *collabStorage) AllFiles(collection *firestore.CollectionRef) ([]collections.FileInfo, error) {
	docs, err := cs.allDocs(collection)
	if err != nil {
		return nil, err
	}
	fileInfos := []collections.FileInfo{}
	for _, doc := range docs {
		if !doc.Exists() {
			continue
		}
		fileInfo := &collections.FileInfo{}
		err = doc.DataTo(fileInfo)
		if err != nil {
			log.Printf("Error while getting file info: %s", err.Error())
			continue
		}
		if fileInfo.Deleted {
			continue
		}
		fileInfos = append(fileInfos, *fileInfo)
	}

	return fileInfos, nil
}

func (cs *collabStorage) AllHubsForUser(userID string) []string {
	docs := cs.users.Query.
		Where(userIDField, "==", userID).
		Where(roleField, "!=", "NONE"). // TODO (itsazhuhere@): move to a common package.
		Documents(context.Background())
	docIDs := []string{}
	for {
		doc, err := docs.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			log.Printf("error getting hub in AllHubsForUser: %s", err.Error())
			continue
		}
		data, err := doc.DataAt(hubPath)
		if err != nil {
			log.Printf("error getting hub in AllHubsForUser: %s", err.Error())
			continue
		}
		if hubName, ok := data.(string); ok {
			docIDs = append(docIDs, hubName)
		} else {
			log.Printf("field hub is not a string for user %s", userID)
		}
	}

	return docIDs
}

func (cs *collabStorage) UpdateUsersHubList(userID, hubName, role string) error {
	docs := cs.users.Query.
		Where(userIDField, "==", userID).
		Where(hubPath, "==", hubName).
		Documents(context.Background())
	var docRef *firestore.DocumentRef
	doc, err := docs.Next()
	if err != nil {
		if err != iterator.Done {
			return err
		}
		// Entry doesn't exist so we create it.
		docRef, err = cs.AddEntry(cs.users, "", collections.UserToHubEntry{
			UserID: userID,
			Hub:    hubName,
		})
		if err != nil {
			return err
		}
	} else {
		docRef = doc.Ref
	}

	return cs.UpdateEntry(docRef, roleField, role)
}
