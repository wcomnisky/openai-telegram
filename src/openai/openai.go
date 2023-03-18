package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/expirymap"
	"github.com/tztsai/openai-telegram/src/sse"
)

const KEY_ACCESS_TOKEN = "accessToken"
const USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"

type Conversation struct {
	Messages []sse.Message
}

type GPT4 struct {
	SessionToken   string
	AccessTokenMap expirymap.ExpiryMap
	conversations  map[int64]Conversation
}

type SessionResult struct {
	Error       string `json:"error"`
	Expires     string `json:"expires"`
	AccessToken string `json:"accessToken"`
}

// type MessageResponse struct {
// 	ConversationId string `json:"conversation_id"`
// 	Error          string `json:"error"`
// 	Message        struct {
// 		ID      string `json:"id"`
// 		Content struct {
// 			Parts []string `json:"parts"`
// 		} `json:"content"`
// 	} `json:"message"`
// }

type MessageResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Message      sse.Message `json:"message"`
		FinishReason string      `json:"finish_reason"`
		Index        int         `json:"index"`
	} `json:"choices"`
}

type ChatResponse struct {
	Message string
}

func Init(config *config.EnvConfig) *GPT4 {
	return &GPT4{
		AccessTokenMap: expirymap.New(),
		SessionToken:   config.OpenAIKey,
		conversations:  make(map[int64]Conversation),
	}
}

func (c *GPT4) IsAuthenticated() bool {
	_, err := c.refreshAccessToken()
	return err == nil
}

func (c *GPT4) EnsureAuth() error {
	_, err := c.refreshAccessToken()
	return err
}

func (c *GPT4) ResetConversation(chatID int64) {
	c.conversations[chatID] = Conversation{}
}

func (c *GPT4) SendMessage(message string, tgChatID int64) (chan ChatResponse, error, []string) {
	accessToken, err := c.refreshAccessToken()
	infos := []string{}

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Couldn't get access token: %v", err)), infos
	}

	// client := sse.Init("https://chat.openai.com/backend-api/conversation")
	client := sse.Init("https://api.openai.com/v1/chat/completions")

	client.Headers = map[string]string{
		"User-Agent":    USER_AGENT,
		"Authorization": fmt.Sprintf("Bearer %s", accessToken),
		"Content-Type":  "application/json",
	}

	convo := c.conversations[tgChatID]

	var msg sse.Message
	if strings.HasPrefix(message, "SYSTEM:") {
		log.Println("Setting system prompt...")
		message = strings.TrimPrefix(message, "SYSTEM:")
		msg = sse.Message{Role: "system", Content: message}
	} else {
		msg = sse.Message{Role: "user", Content: message}
	}

	convo.Messages = append(convo.Messages, msg)

	for len(convo.Messages) > 0 {
		err = client.Connect(convo.Messages)
		if err != nil {
			if strings.Contains(err.Error(), "400 Bad Request") {
				convo.Messages = convo.Messages[2:] // delete both Q & A
				info := "Max tokens exceeded, deleted the earliest message"
				log.Println(info)
				infos = append(infos, "Info: "+info)
				log.Println("Conversation length:", len(convo.Messages))
			} else {
				return nil, errors.New(fmt.Sprintf("Couldn't connect to OpenAI: %v", err)), infos
			}
		} else {
			break
		}
	}

	r := make(chan ChatResponse)

	go func() {
		defer close(r)
	mainLoop:
		for {
			select {
			case chunk, ok := <-client.EventChannel:
				if len(chunk) == 0 || !ok {
					break mainLoop
				}

				var res MessageResponse
				err := json.Unmarshal(chunk, &res)
				if err != nil {
					log.Printf("Couldn't unmarshal message response: %v", err)
					continue
				}

				if len(res.Choices) > 0 {
					msg := res.Choices[0].Message
					log.Printf("Got response from GPT4:\n%s", string(chunk))

					convo.Messages = append(convo.Messages, msg)
					c.conversations[tgChatID] = convo
					log.Println("Conversation length:", len(convo.Messages))

					r <- ChatResponse{Message: msg.Content +
						fmt.Sprintf("\n(tokens: %d => %d)",
							res.Usage.PromptTokens, res.Usage.CompletionTokens)}
				}
			}
		}
	}()

	return r, nil, infos
}

func (c *GPT4) refreshAccessToken() (string, error) {
	return c.SessionToken, nil

	cachedAccessToken, ok := c.AccessTokenMap.Get(KEY_ACCESS_TOKEN)
	if ok {
		return cachedAccessToken, nil
	}

	req, err := http.NewRequest("GET", "https://chat.openai.com/api/auth/session", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Cookie", fmt.Sprintf("__Secure-next-auth.session-token=%s", c.SessionToken))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %v", err)
	}
	defer res.Body.Close()

	var result SessionResult
	err = json.NewDecoder(res.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	accessToken := result.AccessToken
	if accessToken == "" {
		return "", errors.New("unauthorized")
	}

	if result.Error != "" {
		if result.Error == "RefreshAccessTokenError" {
			return "", errors.New("Session token has expired")
		}

		return "", errors.New(result.Error)
	}

	expiryTime, err := time.Parse(time.RFC3339, result.Expires)
	if err != nil {
		return "", fmt.Errorf("failed to parse expiry time: %v", err)
	}
	c.AccessTokenMap.Set(KEY_ACCESS_TOKEN, accessToken, expiryTime.Sub(time.Now()))

	return accessToken, nil
}
