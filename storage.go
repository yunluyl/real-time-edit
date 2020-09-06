package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strconv"

	"google.golang.org/api/iterator"

	"cloud.google.com/go/firestore"
	"github.com/mitchellh/mapstructure"
)

var (
	client              *firestore.Client
	operationCollection *firestore.CollectionRef
)

type OperationEntry struct {
	hub string

	file string

	index int64

	op string
}

func init() {
	client, err := firestore.NewClient(context.Background(), "yunlu-test")
	if err != nil {
		log.Fatal("initiate Firestore client failed: %+v")
	}
	operationCollection = client.Collection("operations")
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
		mapstructure.Decode(doc.Data(), &data)
		if start == -1 {
			start = data.index
			count = data.index
		} else if data.index-count == 1 {
			count++
		} else {
			msg := fmt.Sprintf("Query operation not in sequence prev index: %d, cur index: %d", count, data.index)
			log.Println(msg)
			return statusOperationCommitError, idx, []string{}, msg
		}
		retOps = append(retOps, data.op)
	}
	if start == -1 {
		log.Printf("operation index: %d larger than upper bound", idx)
		return statusOperationTooNew, idx, []string{}, ""
	} else if start == idx-1 && len(retOps) == 1 {
		batch := client.Batch()
		for i, op := range ops {
			operationEntry := &OperationEntry{
				hub:   hubName,
				file:  file,
				index: idx + int64(i),
				op:    op,
			}
			docRef := operationCollection.Doc(generateDocID(operationEntry))
			batch.Create(docRef, operationEntry)
		}
		_, err := batch.Commit(context.Background())
		if err != nil {
			return statusOperationCommitError, idx, []string{}, err.Error()
		}
		return statusOperationCommitted, idx, ops, ""
	} else if start > idx {
		log.Printf("operation index: %d smaller than lower bound: %d", idx, start)
		return statusOperationTooOld, start, retOps, ""
	} else {
		return statusOperationTooOld, idx, retOps[idx-start:], ""
	}
}

func generateDocID(operationEntry *OperationEntry) string {
	return base64.StdEncoding.EncodeToString([]byte(
		operationEntry.hub +
			"###" +
			operationEntry.file +
			"###" +
			strconv.FormatInt(operationEntry.index, 10)))
}

func create(operationEntry *OperationEntry) error {
	docRef := operationCollection.Doc(generateDocID(operationEntry))
	_, err := docRef.Create(context.Background(), operationEntry)
	if err != nil {
		return err
	}
	return nil
}
