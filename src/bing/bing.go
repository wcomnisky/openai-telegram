package bing

import (
	"encoding/json"
	"fmt"
	"strings"

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

func (c *API) Send(query string) (string, error) {
	client := c.InitClient()
	params := map[string]string{"q": query}

	err := client.Connect("GET", params, nil)
	if err != nil {
		return "", err
	}

	chunk, ok := <-client.EventChannel
	if len(chunk) == 0 || !ok {
		return "", fmt.Errorf("no response from Bing")
	}
	var res map[string]any
	err = json.Unmarshal(chunk, &res)
	if err != nil {
		return "", err
	}
	return ExtractResponse(res), nil
}

// TODO shorten the response
func ExtractResponse(resp map[string]any) string {
	for _, k := range [3]string{"computation", "timeZone", "webPages"} {
		a, ok := resp[k]
		if ok {
			if k == "webPages" {
				pages := a.(map[string]any)["value"].([]interface{})
				s := []string{}
				for _, page := range pages[:7] {
					s = append(s, FormatPageSnippet(page.(map[string]any)))
				}
				return strings.Join(s, "\n")
			} else if k == "computation" {
				a := a.(map[string]any)
				exp := a["expression"].(string)
				ans := a["value"].(string)
				return fmt.Sprintf("%s = %s", exp, ans)
			} else if k == "timeZone" {
				a := a.(map[string]any)["primaryCityTime"].(map[string]any)
				return fmt.Sprintf("%s, %s, %s", a["time"], a["utcOffset"], a["location"])
			}
		}
	}
	return ""
}

func FormatPageSnippet(page map[string]interface{}) string {
	return fmt.Sprintf("[%s](%s)\n%s\n", page["name"], page["url"], page["snippet"])
}
