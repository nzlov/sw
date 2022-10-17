package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	node *Node

	cid int

	clientid string
	user     string
	tags     []string

	log *zap.SugaredLogger

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.node.UnRegister(c)
		c.conn.Close()
	}()
	c.conn.SetReadLimit(DefConfig.Client.ReadMessageSizeLimit)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Error(err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.ReplaceAll(message, newline, space))
		c.node.ClientHandler(c, message)
	}
}

func istoss(v []interface{}) []string {
	ss := []string{}
	for _, vv := range v {
		ss = append(ss, fmt.Sprint(vv))
	}
	return ss
}

func resp(rt, i, c, m string) []byte {
	return []byte(`{"t":"r","rt":"` + rt + `","i":"` + i + `","c":` + c + `,"m":"` + m + `"}`)
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

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.log.Errorf("NextWriter:%v\n", err.Error())
				return
			}
			c.log.Infof("Write:%v\n", string(message))
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			// n := len(c.send)
			// for i := 0; i < n; i++ {
			//     w.Write(newline)
			//     message = <-c.send
			//     c.log.Infof("Write:%v\n", string(message))
			//     w.Write(message)
			// }

			if err := w.Close(); err != nil {
				c.log.Errorf("NextWriter Close:%v\n", err.Error())
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.log.Errorf("WriteMessage PingMessage:%v\n", err.Error())
				return
			}
		}
	}
}
