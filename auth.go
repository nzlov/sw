package main

import (
	"crypto/md5"
	"encoding/hex"
)

func CheckTokenMD5(secret, user, m, timestamp, pk string) bool {
	h := md5.New()
	h.Write([]byte(secret + user + m + timestamp))
	return hex.EncodeToString(h.Sum(nil)) == pk
}

func CheckSignMD5(secret, data, timestamp, pk string) bool {
	h := md5.New()
	h.Write([]byte(secret + data + timestamp))
	return hex.EncodeToString(h.Sum(nil)) == pk
}
