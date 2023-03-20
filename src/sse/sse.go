package sse

import (
	"encoding/json"
	"errors"
	"fmt"
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

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature"`
}

func Init(url string) Client {
	return Client{
		URL:          url,
		EventChannel: make(chan []byte),
	}
}

func (c *Client) Connect(request Request) error {
	bs, _ := json.Marshal(&request)
	body := string(bs)

	req, err := http.NewRequest("POST", c.URL, strings.NewReader(body))
	if err != nil {
		return errors.New(fmt.Sprintf("failed to create request: %v", err))
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
