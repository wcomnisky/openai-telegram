package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

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

func (c *GPT4) ResetConversation(chatID int64) {
	c.conversations[chatID] = Conversation{}
}

func (c *GPT4) GetChats() []int64 {
	keys := make([]int64, 0, len(c.conversations))
	for k := range c.conversations {
		keys = append(keys, k)
	}
	return keys
}

func (c *GPT4) ConversationText(chatID int64) string {
	msgs := c.conversations[chatID].Messages
	ms, err := json.Marshal(&msgs)
	if err != nil {
		log.Println(err)
		return ""
	}
	return string(ms)
}

func (c *GPT4) SendMessage(message string, tgChatID int64) (chan ChatResponse, error, []string) {
	accessToken := c.SessionToken
	infos := []string{}

	client := sse.Init("https://api.openai.com/v1/chat/completions")

	client.Headers = map[string]string{
		"User-Agent":    USER_AGENT,
		"Authorization": fmt.Sprintf("Bearer %s", accessToken),
		"Content-Type":  "application/json",
	}

	convo := c.conversations[tgChatID]
	var role string

	var msg sse.Message
	if strings.HasPrefix(message, "SYSTEM:") {
		role = "system"
		log.Println("Set system prompt")
		infos = append(infos, "Set system prompt")
		message = strings.TrimPrefix(message, "SYSTEM:")
	} else {
		role = "user"
	}

	msg = sse.Message{Role: role, Content: message}
	convo.Messages = append(convo.Messages, msg)
	c.conversations[tgChatID] = convo

	for len(convo.Messages) > 0 {
		err := client.Connect(c.ConversationText(tgChatID))
		if err != nil {
			if strings.Contains(err.Error(), "400 Bad Request") {
				convo.Messages = convo.Messages[1:] // delete both Q & A
				info := "Max tokens exceeded, deleted the earliest message"
				log.Println(info)
				infos = append(infos, info)
				log.Println("Conversation length:", len(convo.Messages))
			} else {
				return nil, errors.New(fmt.Sprintf("Couldn't connect to OpenAI: %v", err)), infos
			}
		} else {
			break
		}
	}

	if role == "system" {
		return nil, nil, infos
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
