package main

import "gorm.io/gorm"

type UserTag struct {
	gorm.Model

	UsersID string `json:"usersid" gorm:"column:userid;index"`
	Tag     string `json:"tag" gorm:"column:tag;index"`
}

type Message struct {
	gorm.Model

	MessagesID string `json:"messagesid" gorm:"column:messageid;index"`

	Data string `json:"data" gorm:"column:data"`
}

type UserMessage struct {
	gorm.Model

	MessagesID string `json:"messagesid" gorm:"column:messageid;index"`
	UsersID    string `json:"usersid" gorm:"column:userid;index"`
	Ack        bool   `json:"ack" gorm:"column:ack;index"`
}

type AdminPushMessage struct {
	MessageID string
	UserIDs   []string `json:"us"`
	Tags      []string `json:"ts"`

	Data string `json:"d"`
}

type PushMessageClient struct {
	T  string        `json:"t"`
	Ms []PushMessage `json:"ms"`
}

type ClusterMessage struct {
	NodeName  string
	Message   AdminPushMessage
	Timestamp int64
}

type PushMessage struct {
	ID   string `json:"id"`
	Ts   int64  `json:"ts"`
	Data string `json:"data"`
}

type ClientAck struct {
	User string
	IDs  []string
}
