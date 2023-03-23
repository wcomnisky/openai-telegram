package bing

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/sse"
)

const API_URL = "https://api.bing.microsoft.com/v7.0/search"

type API struct {
	URL string
	Key string
}

func Init(config *config.EnvConfig) *API {
	return &API{
		URL: API_URL,
		Key: config.AzureKey,
	}
}

func (c *API) InitClient() sse.Client {
	client := sse.Init(c.URL)
	client.Headers = map[string]string{
		"Ocp-Apim-Subscription-Key": c.Key,
	}
	return client
}

func (c *API) SendQuery(query string) (string, error) {
	client := c.InitClient()
	params := map[string]string{"q": query}

	err := client.Connect(nil, "GET", params)
	if err != nil {
		return "", err
	}

	r := make(chan string)

	go func() {
		defer close(r)
	mainLoop:
		for {
			select {
			case chunk, ok := <-client.EventChannel:
				if len(chunk) == 0 || !ok {
					break mainLoop
				}

				var res map[string]any
				err := json.Unmarshal(chunk, &res)
				if err != nil {
					log.Printf("Couldn't unmarshal message response: %v", err)
					continue
				}

				r <- ExtractResponse(res)
			}
		}
	}()

	s := <-r
	return s, nil
}

func ExtractResponse(resp map[string]any) string {
	delete(resp, "_type")
	delete(resp, "queryContext")
	delete(resp, "rankingResponse")
	// resp["webPages"] = resp["webPages"]["value"][:5]
	return fmt.Sprint(resp)
}
