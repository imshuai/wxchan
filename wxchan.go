package wxchan

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type commonParms struct {
	Touser                 string `json:"touser"`
	Toparty                string `json:"toparty"`
	Totag                  string `json:"totag"`
	MsgType                string `json:"msgtype"`
	AgentID                int    `json:"agentid"`
	EnableDuplicateCheck   int    `json:"enable_duplicate_check"`
	DuplicateCheckInterval int    `json:"duplicate_check_interval"`
}
type textcard struct {
	commonParms
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	BtnText     string `json:"btntxt"`
}
type Chan struct {
	corpid      string
	agentid     int
	corpsecret  string
	token       string
	expireTimer *time.Timer
}

type msgserializer interface {
	Serialize() (string, error)
}

func (tc *textcard) Serialize() (string, error) {
	byts, err := json.Marshal(tc)
	return string(byts), err
}

func New(corpid, appsecret string, agentid int) (*Chan, error) {
	c := &Chan{
		corpid:     corpid,
		agentid:    agentid,
		corpsecret: appsecret,
	}
	err := c.renew()
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			<-c.expireTimer.C
			c.renew()
		}
	}()
	return c, nil
}

func (c *Chan) renew() error {
	msg := &struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `josn:"expires_in"`
	}{}
	playload := make(map[string]string)
	playload["corpid"] = c.corpid
	playload["corpsecret"] = c.corpsecret
	resp, err := get("https://qyapi.weixin.qq.com/cgi-bin/gettoken", playload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	byts, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(byts, msg)
	if err != nil {
		return err
	}
	if msg.ErrCode != 0 {
		return errors.New(msg.ErrMsg)
	}
	c = &Chan{
		corpid:      c.corpid,
		corpsecret:  c.corpsecret,
		token:       msg.AccessToken,
		expireTimer: time.NewTimer(time.Second * time.Duration(msg.ExpiresIn)),
	}
	return nil
}

func (c *Chan) SendTextCard(title, content, url string) error {
	msgTextCard := &textcard{}
	msgTextCard.Touser = "@all"
	msgTextCard.MsgType = "textcard"
	msgTextCard.AgentID = c.agentid
	msgTextCard.Title = title
	msgTextCard.Description = content
	msgTextCard.URL = url
	resp, err := post("https://qyapi.weixin.qq.com/cgi-bin/message/send", func() (queries map[string]string) {
		queries = make(map[string]string)
		queries["access_token"] = c.token
		return
	}(), msgTextCard)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	msgResp := &struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `josn:"expires_in"`
	}{}
	byts, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(byts, msgResp)
	if err != nil {
		return err
	}
	if msgResp.ErrCode != 0 {
		return errors.New(msgResp.ErrMsg)
	}
	return nil
}

func get(base string, playload map[string]string) (*http.Response, error) {
	url := base + "?"
	for k, v := range playload {
		url = url + k + "=" + v + "&"
	}
	url = strings.TrimRight(url, "&")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	return client.Do(req)
}

func post(base string, queries map[string]string, msg msgserializer) (*http.Response, error) {
	url := base + "?"
	for k, v := range queries {
		url = url + k + "=" + v + "&"
	}
	url = strings.TrimRight(url, "&")
	msgString, err := msg.Serialize()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(msgString))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	return client.Do(req)
}
