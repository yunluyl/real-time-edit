package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strconv"

	"google.golang.org/api/iterator"

	"cloud.google.com/go/firestore"
)

var (
	firestoreClient     *firestore.Client
	operationCollection *firestore.CollectionRef
)

// OperationEntry is the schema of the database entry for transformational operations
type OperationEntry struct {
	Hub string `firestore:"hub"`

	File string `firestore:"file"`

	Index int64 `firestore:"index"`

	Op string `firestore:"op"`
}

func init() {
	var err error
	firestoreClient, err = firestore.NewClient(context.Background(), "yunlu-test")
	if err != nil {
		log.Fatalf("initiate Firestore client failed: %+v", err)
	}
	operationCollection = firestoreClient.Collection("operations")
}

func commitOperations(
	hubName,
	file string,
	idx int64,
	ops []string) (string, int64, []string, string) {
	iter := operationCollection.
		Where("hub", "==", hubName).
		Where("file", "==", file).
		Where("index", ">=", idx-1).
		OrderBy("index", firestore.Asc).
		Documents(context.Background())
	var start int64 = -1
	var count int64 = -1
	retOps := []string{}
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Query operation error: %+v", err)
			return statusOperationCommitError, idx, []string{}, err.Error()
		}
		data := &OperationEntry{}
		convertErr := doc.DataTo(data)
		if convertErr != nil {
			log.Printf("Data conversion error: %+v", err)
			return statusOperationCommitError, idx, []string{}, err.Error()
		}
		if start == -1 {
			start = data.Index
			count = data.Index
		} else if data.Index-count == 1 {
			count++
		} else {
			msg := fmt.Sprintf("Query operation not in sequence prev index: %d, cur index: %d", count, data.Index)
			log.Println(msg)
			return statusOperationCommitError, idx, []string{}, msg
		}
		retOps = append(retOps, data.Op)
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
