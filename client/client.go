package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	addr   = flag.String("addr", "localhost:8080", "http service address")
	user   = flag.String("user", "", "user")
	client = flag.String("client", "", "client")
	secret = flag.String("secret", "", "secret")
	tags   = flag.String("tags", "", "tags. ex: \"a,-b\". tag \"a\" and untag \"b\"")
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

func TokenMD5(secret, user, m, timestamp string) string {
	h := md5.New()
	h.Write([]byte(secret + user + m + timestamp))
	return hex.EncodeToString(h.Sum(nil))
}
func main() {
	flag.Parse()
	log.SetFlags(0)

	if *user == "" || *client == "" || *secret == "" {
		log.Fatalln("no users or no client or no secret")
	}

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	ts := time.Now().Unix()

	c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`
{
    "t":"l",
    "i":"%d",
    "u":"%s",
    "m":"%s",
    "tk":"%s",
    "ts": %d
}
`, time.Now().UnixNano(), *user, *client, TokenMD5(*secret, *user, *client, fmt.Sprint(ts)), ts)))

	if *tags != "" {
		ts := strings.Split(*tags, ",")
		m := map[string]bool{}
		for _, v := range ts {
			if strings.HasPrefix(v, "-") {
				m[v[1:]] = false
			} else {
				m[v] = true
			}

		}
		data, err := json.Marshal(map[string]interface{}{
			"t": "t",
			"i": fmt.Sprint(time.Now().UnixNano()),
			"d": m,
		})
		if err != nil {
			panic(err)
		}

		c.WriteMessage(websocket.TextMessage, data)
	}

	defer close(done)
	go func() {
		nm := NodeMessage{}
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
			ms := bytes.Split(message, newline)
			for _, v := range ms {
				if err := json.Unmarshal(v, &nm); err != nil {
					log.Println("read json:", err)
					continue
				}
				switch nm.Type {
				case "m":
					ids := []string{}
					for _, v := range nm.Messages {
						log.Println("Recive[m]:", v)
						ids = append(ids, v.ID)
					}
					r, err := json.Marshal(ids)
					if err != nil {
						continue
					}

					c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`
{
    "t":"a",
    "i":"%d",
    "id":%s 
}
`, time.Now().UnixNano(), string(r))))
				}

			}
		}
	}()
	select {}
}

type NodeMessage struct {
	Type     string    `json:"t"`
	Messages []Message `json:"ms"`
}
type Message struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}
