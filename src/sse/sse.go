package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	// "github.com/google/uuid"
	// "github.com/launchdarkly/eventsource"
)

type Client struct {
	URL          string
	EventChannel chan []byte
	Headers      map[string]string
}

func Init(url string) Client {
	return Client{
		URL:          url,
		EventChannel: make(chan []byte),
	}
}

func (c *Client) Connect(request any, method string, params map[string]string) error {
	var body io.Reader
	if method == "POST" {
		bs, _ := json.Marshal(&request)
		body = strings.NewReader(string(bs))
	} else {
		body = nil
	}

	req, err := http.NewRequest("POST", c.URL, body)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to create request: %v", err))
	}

	if len(params) > 0 {
		q := req.URL.Query()
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	http := &http.Client{}
	resp, err := http.Do(req)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to connect to SSE: %v", err))
	}
	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("failed to connect to SSE: %v", resp.Status))
	}

	go func() {
		defer resp.Body.Close()
		defer close(c.EventChannel)

		for {
			body, _ := ioutil.ReadAll(resp.Body)

			if err != nil {
				log.Println(errors.New(fmt.Sprintf("failed to decode event: %v", err)))
				break
			}

			c.EventChannel <- body
		}
	}()

	return nil
}
