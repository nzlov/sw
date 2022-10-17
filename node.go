package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Node struct {
	// Registered clients.
	//	clients map[*Client]struct{}
	clients *sync.Map

	//	clientids map[string]*Client
	clientids *sync.Map
	//	users     map[string]map[string]*Client
	users *sync.Map

	db *gorm.DB

	rdb  *redis.Client
	rpub *redis.PubSub

	id int

	upgrader websocket.Upgrader
}

type tag struct {
	c   *Client
	tag map[string]interface{}
}

func newNode() *Node {
	log := zap.S()

	loglevel := logger.Error
	if DefConfig.DBLog {
		loglevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(DefConfig.DB), &gorm.Config{
		CreateBatchSize: 10,
		Logger: logger.New(zap.NewStdLog(zap.L()), logger.Config{
			SlowThreshold: 200 * time.Millisecond,
			LogLevel:      loglevel,
		}),
	})
	if err != nil {
		log.Fatal(err)
	}
	//	db.LogMode(true)
	db.AutoMigrate(new(UserTag), new(Message), new(UserMessage))
	n := &Node{
		clientids: &sync.Map{},
		clients:   &sync.Map{},
		users:     &sync.Map{},
		db:        db,
	}

	n.upgrader = websocket.Upgrader{
		ReadBufferSize:  DefConfig.Client.ReadBufferSize,
		WriteBufferSize: DefConfig.Client.WriteBufferSize,
	}
	n.upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}
	if DefConfig.Redis.Enable {
		n.rdb = redis.NewClient(&redis.Options{
			Addr:         DefConfig.Redis.Host,
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			PoolSize:     10,
			PoolTimeout:  30 * time.Second,
		})
		if DefConfig.Redis.Name == "" {
			DefConfig.Redis.Name = time.Now().Format("Node-20060102150405")
		}
		if DefConfig.Redis.Channel == "" {
			DefConfig.Redis.Channel = DefConfig.Redis.Name
		}

		if err := n.rdb.Ping(context.Background()).Err(); err != nil {
			log.Fatal("redis err:", err.Error())
		}

		go n.clusterRev()

		log.Info("Node Enable Redis Cluster:", DefConfig.Redis.Name, DefConfig.Redis.Channel)

	}

	return n
}

func (n *Node) clusterRev() {
	log := zap.S().With("method", "clusterRev")
	defer func() {
		if err := recover(); err != nil {
			log.Error("ClusterRev err:", err)
		}
		go n.clusterRev()
	}()
	n.rpub = n.rdb.Subscribe(context.Background(), DefConfig.Redis.Channel)

	m := ClusterMessage{}
	for msg := range n.rpub.Channel() {
		if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
			fmt.Printf("ClusterRev Json Error:%+v,%s", msg, err)
			continue
		}
		if m.NodeName == DefConfig.Redis.Name {
			continue
		}
		log.Info("ClusterRev:", DefConfig.Redis.Name, msg.Channel, m.NodeName, m.Message)

		go n.Publish(m.Message, true, m.Timestamp)
	}
}

func (n *Node) Close() {
	if n.rpub != nil {
		n.rpub.Close()
	}
	if n.rdb != nil {
		n.rdb.Close()
	}
}

func (n *Node) Register(client *Client) {
	log := zap.S().With("method", "Register", "user", client.user, "clientid", client.clientid)
	log.Info("register")
	n.clients.Store(client, nil)
	if us, ok := n.users.Load(client.user); ok {
		us.(map[string]*Client)[client.clientid] = client
		n.users.Store(client.user, us)
	} else {
		n.users.Store(client.user, map[string]*Client{
			client.clientid: client,
		})
	}
	// 发送离线消息
	mids := []string{}
	if err := n.db.Model(new(UserMessage)).
		Where("userid = ? and ack = ?", client.user, false).
		Order("addtime").
		Pluck("messageid", &mids).Error; err != nil {
		log.Error("db:find offline message id:", err)
	}
	if len(mids) > 0 {
		skip := 0
		for {
			ms := []Message{}
			end := skip + 5
			if end > len(mids) {
				end = len(mids)
			}
			ids := mids[skip:end]
			if err := n.db.Where("messageid in (?)", ids).Order("addtime").Find(&ms).Error; err != nil {
				log.Error("db:find offline message:", err)
				break
			} else {
				if len(ms) == 0 {
					break
				}
				p := PushMessageClient{
					T:  "m",
					Ms: []PushMessage{},
				}
				for _, v := range ms {
					p.Ms = append(p.Ms, PushMessage{
						ID:   v.MessagesID,
						Ts:   v.CreatedAt.Unix(),
						Data: v.Data,
					})
				}
				data, err := json.Marshal(&p)
				if err != nil {
					log.Error("json:marshal message:", err)
				}
				client.send <- data
				skip += 5
				if skip >= len(mids) {
					break
				}
			}
		}
	}
}

func (n *Node) UnRegister(client *Client) {
	zap.S().Info("unregister:", client.user, client.clientid)
	if _, ok := n.clients.Load(client); ok {
		n.clients.Delete(client)
		if users, ok := n.users.Load(client.user); ok {
			delete(users.(map[string]*Client), client.clientid)
		}
		close(client.send)
	}
}

func (n *Node) Tager(c *Client, tag map[string]interface{}) {
	log := zap.S().With("method", "tager", "user", c.user, "clientid", c.clientid)
	log.Info("Tager")
	nt := []string{}
	ct := []string{}
	for k, v := range tag {
		if vb, ok := v.(bool); ok {
			if vb {
				nt = append(nt, k)
			} else {
				ct = append(ct, k)
			}
		}
	}
	if len(nt) > 0 {
		tags := []string{}
		if err := n.db.Model(new(UserTag)).Where("userid = ? and tag in (?)", c.user, nt).Pluck("tag", &tags).Error; err != nil {
			log.Error("db:find tags users:", err)
		}
		tm := map[string]struct{}{}
		for _, v := range tags {
			tm[v] = struct{}{}
		}
		for _, v := range nt {
			if _, ok := tm[v]; !ok {
				if err := n.db.Create(&UserTag{
					UsersID: c.user,
					Tag:     v,
				}); err != nil {
					log.Error("db:add user_tags users:", c.user, v)
				}
			}
		}
	}
	if len(ct) > 0 {
		if err := n.db.Exec("delete from user_tags where userid = ? and tag in (?)", c.user, ct).Error; err != nil {
			log.Error("db:delete user_tags users:", c.user, ct, err)
		}
	}
}

func (n *Node) Publish(m AdminPushMessage, r bool, ts int64) {
	log := zap.S().With("method", "public")
	log.Info("publish:", m.UserIDs, m.MessageID, m.Tags, m.Data)
	// 查询 tags对应user
	users := []string{}
	if m.Tags != nil && len(m.Tags) > 0 {
		if err := n.db.Model(new(UserTag)).Where("tag in (?)", m.Tags).Pluck("userid", &users).Error; err != nil {
			log.Error("db:find tags users:", err)
		}
	}
	users = sm(users, m.UserIDs)
	if !r {
		// 保存消息
		dm := Message{
			MessagesID: m.MessageID,
			Data:       m.Data,
		}
		if err := n.db.Create(&dm).Error; err != nil {
			log.Error("db:save message:", err)
		}
		ts = dm.CreatedAt.Unix()

		if n.rdb != nil {
			d, err := json.Marshal(ClusterMessage{
				NodeName:  DefConfig.Redis.Name,
				Timestamp: ts,
				Message:   m,
			})
			if err != nil {
				log.Error("redis json:", err.Error())
			} else {
				r, err := n.rdb.Publish(context.Background(), DefConfig.Redis.Channel, string(d)).Result()
				log.Info("redis:", r, err)
			}
		}
	}
	p := PushMessageClient{
		T: "m",
		Ms: []PushMessage{
			{
				ID:   m.MessageID,
				Ts:   ts,
				Data: m.Data,
			},
		},
	}
	data, err := json.Marshal(&p)
	if err != nil {
		log.Error("json:marshal message:", err)
	}
	// 保存发送消息
	for _, id := range users {
		if err := n.db.Create(&UserMessage{
			MessagesID: m.MessageID,
			UsersID:    id,
		}).Error; err != nil {
			log.Error("db:save user message:", err)
		}
		if um, ok := n.users.Load(id); ok {
			for _, c := range um.(map[string]*Client) {
				c.send <- data
			}
		}
	}
}

func (n *Node) Acker(a ClientAck) {
	log := zap.S().With("method", "acker", "user", a.User)
	log.Info("acker", a.IDs)
	if err := n.db.Model(new(UserMessage)).Where("userid = ? and messageid in (?)", a.User, a.IDs).Update("ack", true).Error; err != nil {
		log.Error("acker:db:update user message ack:", err)
	}
}

func (n *Node) auth(c *Client, u, m, tk string, ts int64) bool {
	return CheckTokenMD5(DefConfig.Secret, u, m, fmt.Sprint(ts), tk)
}

func sm(s ...[]string) []string {
	m := map[string]struct{}{}

	for _, ss := range s {
		for _, sss := range ss {
			m[sss] = struct{}{}
		}
	}

	r := []string{}
	for k := range m {
		r = append(r, k)
	}
	return r
}

type ch struct {
	c    *Client
	data []byte
}

func (n *Node) ClientHandler(c *Client, data []byte) {
	m := map[string]interface{}{}
	defer func() {
		if err := recover(); err != nil {
			c.log.Errorf("handler panic:%v\n", err)
			c.send <- resp("e", fmt.Sprint(m["i"]), C_FAIL, fmt.Sprint(err))
		}
	}()
	c.log.Infof("handler:New Message: %+v\n", string(data))

	if err := json.Unmarshal(data, &m); err != nil {
		c.log.Errorf("handler:json unmarshal: %+v\n", err.Error())
		return
	}

	switch m["t"] {
	case "l":
		if c.user != "" {
			c.send <- resp("l", m["i"].(string), C_FAIL, "")
			return
		}

		user := strings.TrimSpace(m["u"].(string))
		clientid := strings.TrimSpace(m["m"].(string))
		if user == "" || clientid == "" {
			c.send <- resp("l", m["i"].(string), C_FAIL, "no user or clientid")
			return
		}
		if !n.auth(c, user, clientid, m["tk"].(string), int64(m["ts"].(float64))) {
			c.send <- resp("l", m["i"].(string), C_AUTH, "")
			return
		}
		c.user = user
		c.clientid = clientid

		c.log = zap.S().With(
			"cid", c.cid,
			"user", c.user,
			"clientid", c.clientid,
		)
		c.send <- resp("l", m["i"].(string), C_OK, c.clientid)
		n.Register(c)
		return
	case "a":
		if c.user == "" {
			c.send <- resp("a", m["i"].(string), C_AUTH, "")
			return
		}

		n.Acker(ClientAck{
			User: c.user,
			IDs:  istoss(m["id"].([]interface{})),
		})
	case "t":
		if c.user == "" {
			c.send <- resp("a", m["i"].(string), C_AUTH, "")
			return
		}
		n.Tager(c, m["d"].(map[string]interface{}))
		c.send <- resp("a", m["i"].(string), C_OK, "")
	default:
		if c.user == "" {
			c.send <- resp("a", m["i"].(string), C_AUTH, "")
			return
		}
		c.log.Errorf("handler error: unknown type:%v\n", m["t"])
		return
	}
}

// serveWs handles websocket requests from the peer.
func (n *Node) serveWs(node *Node, w http.ResponseWriter, r *http.Request) {
	conn, err := n.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	n.id++
	client := &Client{
		cid:  n.id,
		node: node,
		conn: conn,
		send: make(chan []byte, 5),
		log:  zap.S().With("cid", n.id),
	}
	if DefConfig.Client.Compression {
		client.conn.EnableWriteCompression(true)
		client.conn.SetCompressionLevel(DefConfig.Client.CompressionLevel)
	}
	client.conn.SetCloseHandler(func(code int, text string) error {
		client.log.Info("CloseHandler:", code, text)
		message := websocket.FormatCloseMessage(code, "")
		conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(writeWait))
		return nil
	})
	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
