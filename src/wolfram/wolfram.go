package wolfram

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/sse"
)

const API_URL = "http://api.wolframalpha.com/v2/query"

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
	return sse.Init(c.URL)
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
	// resp = resp["queryresult"]
	// if !resp["success"] {
	// 	return "Failed"
	// }
	return fmt.Sprint(resp["pods"])
}
