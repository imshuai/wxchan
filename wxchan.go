package wxchan

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
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

type msgTextcard struct {
	commonParms
	TextCard struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		URL         string `json:"url"`
		BtnText     string `json:"btntxt"`
	} `json:"textcard"`
}
type WXChan struct {
	corpid          string
	agentid         int
	corpsecret      string
	token           string
	tokenExpireTime time.Time
	cache           string
}

func (c *WXChan) isTokenExpired() bool {
	return time.Now().Local().After(c.tokenExpireTime)
}

type msgserializer interface {
	Serialize() (string, error)
}

func (mt *msgTextcard) Serialize() (string, error) {
	byts, err := json.Marshal(mt)
	return string(byts), err
}

func New(corpid, appsecret string, agentid int, cacheFilePath string) (*WXChan, error) {
	c := &WXChan{
		corpid:     corpid,
		agentid:    agentid,
		corpsecret: appsecret,
		token:      "",
		cache:      cacheFilePath,
	}
	byts, err := ioutil.ReadFile(c.cache)
	if os.IsPermission(err) {
		return nil, err
	}
	if err == nil {
		s := strings.Split(string(byts), ",")
		c.token = s[0]
		c.tokenExpireTime = func() time.Time {
			t, _ := time.Parse("2006-01-02 15:04:05", s[1])
			return t
		}()
		if !c.isTokenExpired() {
			return c, nil
		}
	}

	err = c.renew()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *WXChan) renew() error {
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
	c.token = msg.AccessToken
	c.tokenExpireTime = time.Now().Add(time.Second * time.Duration(msg.ExpiresIn))
	ioutil.WriteFile(c.cache, []byte(strings.Join([]string{c.token, c.tokenExpireTime.Format("2006-01-02 15:04:05")}, ",")), os.FileMode(os.O_CREATE|os.O_TRUNC|os.O_SYNC))
	return nil
}

func (c *WXChan) SendTextCard(title, content, url string) error {
	if c.isTokenExpired() {
		c.renew()
	}
	msgTextCard := &msgTextcard{}
	msgTextCard.Touser = "@all"
	msgTextCard.MsgType = "textcard"
	msgTextCard.AgentID = c.agentid
	msgTextCard.TextCard.Title = title
	msgTextCard.TextCard.Description = content
	msgTextCard.TextCard.URL = url
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
		switch msgResp.ErrCode {
		case 40014, 42001:
			c.renew()
			return c.SendTextCard(title, content, url)
		default:
			return errors.New(msgResp.ErrMsg)
		}
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
