package main

import "log"

var db []string
var lo int64 = 0
var hi int64 = 0

func commitOperations(idx int64, ops []string) (string, int64, []string) {
	if idx == hi {
		for _, op := range ops {
			db = append(db, op)
		}
		hi = int64(len(db))
		return statusOperationCommitted, idx, ops
	} else if idx < lo {
		log.Printf("operation index: %d smaller than lower bound: %d", idx, lo)
		return statusOperationTooOld, lo, fetchOperations(lo)
	} else if idx < hi {
		return statusOperationTooOld, idx, fetchOperations(idx)
	} else {
		log.Printf("operation index: %d larger than upper bound: %d", idx, hi)
		return statusOperationTooNew, hi, []string{}
	}
}

func fetchOperations(start int64) []string {
	if start > hi {
		return []string{}
	}
	ret := append([]string{}, db[start:hi]...)
	return ret
}

