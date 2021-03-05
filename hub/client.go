package hub

import (
	log "collabserver/cloudlog"
	"errors"
	"fmt"

	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 9223372036854775807
)

var (
	errNoHub         = errors.New("client is not connected to a hub")
	errNoBackendChan = errors.New("client does not have a backend channel assigned")
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	userID string

	// Send self through this channel to disconnect from the hub.
	unregister chan *Client

	// The channel to send messages from the client to.
	toBackend chan *Message

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan *Message

	// Used as an indication from the hub to stop sending on toBackend. Follows the general rule of
	// not sending on closed channels (which causes program crashing panics).
	// When this chan is closed, all reads will resolve immediately with the second return equaling false.
	stopCh chan struct{}

	closed bool
}

// IsClosed returns true if the client is closed and shouldn't be interacted with anymore.
func (c *Client) IsClosed() bool {
	return c.closed
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.Unregister()
		c.conn.Close()
		c.closed = true
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		var message Message
		err := c.conn.ReadJSON(&message)
		if err != nil {
			log.Printf("read pump error %+v", err)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message.client = c
		err = c.clientToBackend(&message)
		if err != nil {
			log.Printf("error sending %#v to backend: %s", message, err.Error())
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		c.closed = true
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.conn.WriteJSON(message)
			if err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Start begins the read and write goroutines for hub <-> client communication.
func (c *Client) Start() {
	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go c.writePump()
	go c.readPump()
}

// Unregister removes the client from the current hub.
func (c *Client) Unregister() error {
	if c.unregister == nil {
		return errNoHub
	}
	// We check stopCh twice to ensure that we read a stop request before attempting to read
	// from a potentially closed chan (c.toBackend)
	select {
	case <-c.stopCh:
		return fmt.Errorf("client received stop send request")
	default:
	}
	select {
	case <-c.stopCh:
		return fmt.Errorf("client received stop send request")
	case c.unregister <- c:
	}

	return nil
}

// Assign the channels that the Client will need to use for communication with a hub.
func (c *Client) assignChans(backendChan chan *Message, stopCh chan struct{}) {
	c.toBackend = backendChan
	c.stopCh = stopCh
}

func (c *Client) clientToBackend(message *Message) error {
	if c.toBackend == nil {
		return errNoBackendChan
	}
	// We check stopCh twice to ensure that we read a stop request before attempting to read
	// from a potentially closed chan (c.toBackend)
	select {
	case <-c.stopCh:
		return fmt.Errorf("client received stop send request")
	default:
	}
	select {
	case <-c.stopCh:
		return fmt.Errorf("client received stop send request")
	case c.toBackend <- message:
	}

	return nil
}

// NewClient returns a newly instantiated client. Hub is not assigned and will need to be in order to
// perform hub related actions.
func NewClient(userID string, conn *websocket.Conn) *Client {
	return &Client{userID: userID, conn: conn, send: make(chan *Message, 256)}
}
