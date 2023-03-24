package sse

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
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
	var err error

	if method == "POST" {
		bs, _ := json.Marshal(&request)
		body = strings.NewReader(string(bs))
	} else {
		body = nil
	}

	req, err := http.NewRequest(method, c.URL, body)
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

	var resp *http.Response

	for i := 0; i < 5; i++ {
		http := &http.Client{}
		resp, err = http.Do(req)
		if err != nil {
			break
		} else if resp.StatusCode == 429 || resp.StatusCode == 400 {
			log.Printf("failed to connect to SSE, retry %d/5", i+1)
			i, _ := rand.Int(rand.Reader, big.NewInt(3000))
			k := i.Int64() + 1000
			time.Sleep(time.Duration(k) * time.Millisecond)
		} else {
			break
		}
	}

	if err != nil {
		return err
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
