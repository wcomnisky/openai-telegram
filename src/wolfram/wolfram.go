package wolfram

import (
	"encoding/json"
	"fmt"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/sse"
)

const API_URL = "http://api.wolframalpha.com/v2/query"
const USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"

type API struct {
	URL    string
	AppID  string
	Format string
	Output string
}

func Init(config *config.EnvConfig) *API {
	return &API{
		URL:    API_URL,
		AppID:  config.WolframAppID,
		Format: "plaintext",
		Output: "JSON",
	}
}

func (c *API) InitClient() sse.Client {
	client := sse.Init(c.URL)
	client.Headers = map[string]string{
		"User-Agent": USER_AGENT,
	}
	return client
}

func (c *API) SendQuery(query string) (string, error) {
	client := c.InitClient()
	params := map[string]string{
		"input":  query,
		"format": c.Format,
		"output": c.Output,
		"appid":  c.AppID,
	}

	err := client.Connect(nil, "GET", params)
	if err != nil {
		return "", err
	}

	chunk, ok := <-client.EventChannel
	if len(chunk) == 0 || !ok {
		return "", fmt.Errorf("No response from WolframAlpha")
	}
	var res map[string]map[string]any
	err = json.Unmarshal(chunk, &res)
	if err != nil {
		return "", err
	}
	return ExtractResponse(res), nil
}

func ExtractResponse(resp map[string]map[string]any) string {
	res := resp["queryresult"]
	ans, ok := res["pods"]
	if !ok {
		return "Failed"
	} else {
		return fmt.Sprint(ans)
	}
}
