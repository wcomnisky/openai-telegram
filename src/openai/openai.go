package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/tztsai/openai-telegram/src/bing"
	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/sse"
	"github.com/tztsai/openai-telegram/src/wolfram"
)

const API_URL = "https://api.openai.com/v1/chat/completions"
const KEY_ACCESS_TOKEN = "accessToken"
const USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"

const MAX_TOKENS = 8192

type Conversation struct {
	Messages    []Message
	TotalTokens int
	Verbose     bool
}

type GPT4 struct {
	ModelName     string
	SessionToken  string
	Conversations map[int64]Conversation
	Temperature   float32
	Bing          bing.API
	Wolfram       wolfram.API
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature"`
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
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
		Index        int     `json:"index"`
	} `json:"choices"`
}

type ChatResponse struct {
	Message string
}

func Init(config *config.EnvConfig) *GPT4 {
	return &GPT4{
		ModelName:     "gpt-4",
		SessionToken:  config.OpenAIKey,
		Conversations: make(map[int64]Conversation),
		Temperature:   0.7,
	}
}

func (c *GPT4) ResetConversation(chatID int64) {
	c.Conversations[chatID] = Conversation{}
}

func (c *GPT4) GetChats() []int64 {
	keys := make([]int64, 0, len(c.Conversations))
	for k := range c.Conversations {
		keys = append(keys, k)
	}
	return keys
}

func (c *GPT4) InitClient() sse.Client {
	client := sse.Init(API_URL)
	client.Headers = map[string]string{
		"User-Agent":    USER_AGENT,
		"Authorization": fmt.Sprintf("Bearer %s", c.SessionToken),
		"Content-Type":  "application/json",
	}
	return client
}

func (c *GPT4) MakeRequest(messages []Message) Request {
	return Request{
		Model:       c.ModelName,
		Messages:    messages,
		Temperature: c.Temperature,
	}
}

func (c *GPT4) SendMessage(message string, tgChatID int64) (chan ChatResponse, error, []string) {
	var infos = []string{}
	var role string
	var msg Message
	var err error

	client := c.InitClient()

	convo := c.Conversations[tgChatID]

	if strings.HasPrefix(message, "SYSTEM:") {
		role = "system"
		log.Println("Set system prompt")
		message = strings.TrimSpace(strings.TrimPrefix(message, "SYSTEM:"))
		infos = append(infos, "Added system prompt")
	} else {
		role = "user"
	}

	msg = Message{Role: role, Content: message}
	convo.Messages = append(convo.Messages, msg)

	if role == "system" {
		c.Conversations[tgChatID] = convo
		return nil, nil, infos
	}

	tokens_thresh := int(math.Round(MAX_TOKENS * 0.9))

	if convo.TotalTokens > tokens_thresh {
		log.Println("Conversation getting too long, deleted earliest responses")
		for i, c := 0, 0; i < len(convo.Messages)-6 && c < 2; i++ {
			if msg.Role == "assistant" {
				convo.Messages = append(convo.Messages[:i], convo.Messages[i+1:]...)
				c++
			}
		}
	}

	for tries := 1; len(convo.Messages) > 0; tries++ {
		err = client.Connect(c.MakeRequest(convo.Messages), "POST", make(map[string]string))

		if err != nil {
			if tries < 2 && strings.Contains(err.Error(), "400 Bad Request") && len(convo.Messages) > 4 {
				convo.Messages = convo.Messages[2:]
				info := "Max tokens exceeded, deleted earliest messages"
				infos = append(infos, info)
				log.Println(info)
				log.Println("Conversation length:", len(convo.Messages))
				time.Sleep(1 * time.Second)
			} else {
				return nil, err, infos
			}
		} else {
			break
		}
	}

	r := make(chan ChatResponse)

	go func() {
		defer close(r)
		for {
			select {
			case chunk, ok := <-client.EventChannel:
				if len(chunk) == 0 {
					return
				} else if !ok {
					err = errors.New("Bad response from OpenAI")
					return
				}

				var res MessageResponse
				err = json.Unmarshal(chunk, &res)
				if err != nil {
					log.Printf("Couldn't unmarshal message response: %v", err)
					continue
				}

				if len(res.Choices) > 0 {
					msg := res.Choices[0].Message
					log.Printf("Got response from GPT4:\n%s", string(chunk))

					tok_in, tok_out := res.Usage.PromptTokens, res.Usage.CompletionTokens
					convo.Messages = append(convo.Messages, msg)
					convo.TotalTokens = tok_in + tok_out
					log.Println("Conversation length:", len(convo.Messages))

					r <- ChatResponse{Message: msg.Content}

					infos = append(infos, fmt.Sprintf("Tokens: %d => %d", tok_in, tok_out))

					if strings.HasPrefix(msg.Content, "ℹ️ Ask ") {
						var ans string
						if strings.HasPrefix(msg.Content, "ℹ️ Ask Bing") {
							ans, err = c.Bing.SendQuery(msg.Content)
						} else if strings.HasPrefix(msg.Content, "ℹ️ Ask Wolfram") {
							ans, err = c.Wolfram.SendQuery(msg.Content)
						}
						if err != nil {
							return
						}
						convo.Messages = append(convo.Messages,
							Message{Role: "assistant", Content: ans})
					}
					c.Conversations[tgChatID] = convo
				}
			}
		}
	}()

	return r, err, infos
}

// func (c *Client) AskBing(query string) error {

// }
