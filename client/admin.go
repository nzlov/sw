package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	addr   = flag.String("addr", "localhost:8000", "addr")
	secret = flag.String("secret", "", "secret")
	tags   = flag.String("tags", "", "tag list")
	users  = flag.String("users", "", "users list")
	msg    = flag.String("msg", "", "message")
)

func main() {
	flag.Parse()
	fmt.Println(SWPublishTo(strings.Split(*users, ","), strings.Split(*tags, ","), *msg))
}

func SWPublishTo(ids, tags []string, obj string) (string, error) {
	return SWPublish(map[string]interface{}{
		"d":  obj,
		"us": ids,
		"ts": tags,
		"e":  nil,
	})
}

func MD5(s string) string {
	m := md5.New()
	m.Write([]byte(s))
	return hex.EncodeToString(m.Sum(nil))
}

func SWPublish(m map[string]interface{}) (string, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	client := &http.Client{}
	params := url.Values{}
	Url, err := url.Parse(*addr)
	if err != nil {
		return "", err
	}

	md, err := json.Marshal(m)
	if err != nil {
		return "", err
	}

	params.Set("sign", MD5(*secret+string(md)+ts))
	params.Set("ts", ts)
	Url.RawQuery = params.Encode()

	urlPath := Url.String()
	req, err := http.NewRequest("GET", urlPath, strings.NewReader(string(md)))
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	result := Result{}
	err = json.Unmarshal([]byte(body), &result)
	if err != nil {
		return "", err
	}

	return result.Code, err
}

type Result struct {
	Code string `json:"code"`
	Data string `json:"data"`
}
