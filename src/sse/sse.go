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

func Init(url string) Client {
	return Client{
		URL:          url,
		EventChannel: make(chan []byte),
	}
}

func (c *Client) Connect(messages []Message) error {
	// messages, err := json.Marshal([]string{message})
	// if err != nil {
	// 	return errors.New(fmt.Sprintf("failed to encode message: %v", err))
	// }

	// if parentMessageId == "" {
	// 	parentMessageId = uuid.NewString()
	// }

	// var conversationIdString string
	// if conversationId != "" {
	// 	conversationIdString = fmt.Sprintf(`, "conversation_id": "%s"`, conversationId)
	// }

	// if conversation id is empty, don't send it
	// body := fmt.Sprintf(`{
	//     "action": "next",
	//     "messages": [
	//         {
	//             "id": "%s",
	//             "role": "user",
	//             "content": {
	//                 "content_type": "text",
	//                 "parts": %s
	//             }
	//         }
	//     ],
	//     "model": "text-davinci-002-render",
	// 	"parent_message_id": "%s"%s
	// }`, uuid.NewString(), string(messages), parentMessageId, conversationIdString)

	ms, err := json.Marshal(&messages)

	body := fmt.Sprintf(`{
		"model": "gpt-4",
		"messages": %s,
		"temperature": 0.7
	}`, string(ms))

	req, err := http.NewRequest("POST", c.URL, strings.NewReader(body))
	if err != nil {
		return errors.New(fmt.Sprintf("failed to create request: %v", err))
	}

	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	// req.Header.Set("Accept", "text/event-stream")
	// req.Header.Set("Content-Type", "application/json")

	http := &http.Client{}
	resp, err := http.Do(req)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to connect to SSE: %v", err))
	}

	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("failed to connect to SSE: %v", resp.Status))
	}

	// decoder := eventsource.NewDecoder(resp.Body)

	go func() {
		defer resp.Body.Close()
		defer close(c.EventChannel)

		for {
			// event, err := decoder.Decode()
			body, _ := ioutil.ReadAll(resp.Body)

			if err != nil {
				log.Println(errors.New(fmt.Sprintf("failed to decode event: %v", err)))
				break
			}

			// if event.Data() == "[DONE]" || event.Data() == "" {
			// 	break
			// }

			c.EventChannel <- body // event.Data()
		}
	}()

	return nil
}
