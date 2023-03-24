package bing

import (
	"encoding/json"
	"fmt"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/sse"
)

const API_URL = "https://api.bing.microsoft.com/v7.0/search"
const USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"

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
		"User-Agent":                USER_AGENT,
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

	chunk, ok := <-client.EventChannel
	if len(chunk) == 0 || !ok {
		return "", fmt.Errorf("No response from Bing")
	}
	var res map[string]any
	err = json.Unmarshal(chunk, &res)
	if err != nil {
		return "", err
	}
	return ExtractResponse(res), nil
}

func ExtractResponse(resp map[string]any) string {
	for _, k := range [3]string{"computation", "timeZone", "webPages"} {
		a, ok := resp[k]
		if ok {
			if k == "webPages" {
				a := a.(map[string]any)
				pages := a["value"].([]interface{})
				return fmt.Sprint(pages[:3])
			}
			return fmt.Sprint(a)
		}
	}
	return ""
}
