package wolfram

import (
	"encoding/json"
	"fmt"
	"strings"

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
		Format: "image,plaintext",
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

func (c *API) Send(query string) (string, error) {
	client := c.InitClient()
	params := map[string]string{
		"input":  query,
		"format": c.Format,
		"output": c.Output,
		"appid":  c.AppID,
	}

	err := client.Connect("GET", params, nil)
	if err != nil {
		return "", err
	}

	chunk, ok := <-client.EventChannel
	if len(chunk) == 0 || !ok {
		return "", fmt.Errorf("no response from WolframAlpha")
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
		return ""
	} else {
		pods := []string{}
		for _, pod := range ans.([]interface{}) {
			pods = append(pods, FormatWolframPod(pod.(map[string]any)))
		}
		return strings.Join(pods, "")
	}
}

func FormatWolframPod(pod map[string]any) string {
	if pod["error"].(bool) {
		return "(Error)"
	}
	res := "# " + pod["title"].(string) + "\n"
	subpods := []string{}
	for _, subpod := range pod["subpods"].([]interface{}) {
		s := subpod.(map[string]any)["plaintext"].(string)
		if s != "" {
			subpods = append(subpods, s+"\n")
		}
	}
	if len(subpods) > 0 {
		res += strings.Join(subpods, "")
	} else {
		return ""
	}
	infos, ok := pod["infos"]
	if ok {
		tmp, ok := infos.([]interface{})
		if !ok {
			text, ok := infos.(map[string]any)["text"]
			if ok {
				res += text.(string) + "\n"
			}
		} else {
			for _, info := range tmp {
				text, ok := info.(map[string]any)["text"]
				if ok {
					res += text.(string) + "\n"
				}
			}
		}
	}
	return res + "\n"
}
