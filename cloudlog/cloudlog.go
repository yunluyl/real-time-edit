// Package cloudlog takes care of setting up a Google Cloud logger.
package cloudlog

import (
	"context"
	"log"

	logging "cloud.google.com/go/logging"
)

const (
	projectID = "yunlu-test"
	logName   = "collab_info"
)

var (
	// Logger is an already set up instance of *log.Logger
	Logger *log.Logger

	working bool
)

func init() {
	client, err := logging.NewClient(context.Background(), projectID)
	if err != nil {
		log.Print("Failed to create logging client")
		return
	}

	Logger = client.Logger(logName).StandardLogger(logging.Info)
	working = true

}

// Print is a proxy for Logger.Print
func Print(v ...interface{}) {
	log.Print(v...)
	if working {
		Logger.Print(v...)
	}
}

// Println is a proxy for Logger.Println
func Println(v ...interface{}) {
	log.Println(v...)
	if working {
		Logger.Println(v...)
	}
}

// Printf is a proxy for Logger.Println
func Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
	if working {
		Logger.Printf(format, v...)
	}
}

// Fatal is a proxy for Logger.Fatal
func Fatal(v ...interface{}) {
	if working {
		Logger.Fatal(v...)
	}
	log.Fatal(v...)
}

// Fatalf is a proxy for Logger.Fatalf
func Fatalf(format string, v ...interface{}) {
	if working {
		Logger.Fatalf(format, v...)
	}
	log.Fatalf(format, v...)
}
