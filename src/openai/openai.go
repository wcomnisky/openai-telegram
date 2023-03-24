package openai

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
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
	Time        time.Time
}

type GPT4 struct {
	ModelName     string
	SessionToken  string
	Conversations map[int64]Conversation
	Temperature   float32
	Bing          *bing.API
	Wolfram       *wolfram.API
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

// type ChatResponse struct {
// 	Message string
// }

func Init(config *config.EnvConfig) *GPT4 {
	return &GPT4{
		ModelName:     "gpt-4",
		SessionToken:  config.OpenAIKey,
		Conversations: make(map[int64]Conversation),
		Temperature:   1.2,
		Bing:          bing.Init(config),
		Wolfram:       wolfram.Init(config),
	}
}

func (c *GPT4) ResetConversation(chatID int64) {
	delete(c.Conversations, chatID)
}

func (c *GPT4) GetConversation(chatID int64) Conversation {
	convo, ok := c.Conversations[chatID]
	if !ok {
		convo.Time = time.Now()
		c.Conversations[chatID] = convo
	}
	return convo
}

func (c *GPT4) GetChatIDs() []int64 {
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

func (c *GPT4) SendMessage(message string, tgChatID int64) (chan string, error) {
	var role string
	var msg Message
	var err error

	client := c.InitClient()

	convo := c.Conversations[tgChatID]

	r := make(chan string)

	if strings.HasPrefix(message, "SYSTEM:") {
		role = "system"
		log.Println("Set system prompt")
		message = strings.TrimSpace(strings.TrimPrefix(message, "SYSTEM:"))
		// r <- "Added system prompt"
	} else if strings.HasPrefix(message, "TEMPER:") {
		s := strings.TrimSpace(strings.TrimPrefix(message, "TEMPER:"))
		t, err := strconv.ParseFloat(s, 32)
		if err == nil {
			c.Temperature = float32(t)
		}
		return nil, err
	} else {
		role = "user"
	}

	msg = Message{Role: role, Content: message}
	convo.Messages = append(convo.Messages, msg)

	if role == "system" {
		c.Conversations[tgChatID] = convo
		return nil, nil
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
		err = client.Connect(c.MakeRequest(convo.Messages), "POST", map[string]string{})

		if err != nil {
			if tries < 2 && strings.Contains(err.Error(), "400 Bad Request") && len(convo.Messages) > 8 {
				convo.Messages = convo.Messages[2:]
				info := "ℹ️ Max tokens exceeded, deleted earliest messages"
				// r <- info
				log.Println(info)
				log.Println("Conversation length:", len(convo.Messages))
			} else {
				return nil, err
			}
		} else {
			break
		}
	}

	var wait = 0

	go func() {
		defer close(r)
		for {
			select {
			case chunk, ok := <-client.EventChannel:
				if !ok {
					return
				} else if len(chunk) == 0 {
					if wait > 0 {
						continue
					} else {
						return
					}
				}

				var res MessageResponse
				err = json.Unmarshal(chunk, &res)
				if err != nil {
					log.Printf("Couldn't unmarshal message response: %v", err)
					return
				}

				if len(res.Choices) > 0 {
					msg := res.Choices[0].Message
					msg.Content = strings.ReplaceAll(msg.Content, "\r\n", "\n")
					log.Printf("Got response from GPT4:\n%s", string(chunk))

					var text string

					re := regexp.MustCompile(`I ASK (\w+):\s+(.*)`)
					m := re.FindStringSubmatch(msg.Content)

					if wait == 2 {
						s := re.Split(msg.Content, 2)[0]
						msg.Content = fmt.Sprintf("I FOUND:\n\n%s", s)
						convo.Messages[len(convo.Messages)-1] = msg
						wait = 1

						req := c.MakeRequest(convo.Messages)
						for tries := 1; tries < 3; tries++ {
							err = client.Connect(req, "POST", map[string]string{})
							if err == nil {
								break
							}
							time.Sleep(2 * time.Second)
						}
						if err != nil {
							log.Printf("Couldn't send message: %v", err)
							return
						}
					} else {
						convo.Messages = append(convo.Messages, msg)
						wait = 0
					}

					if len(m) > 0 || wait == 1 {
						text = "🤖 " + msg.Content
					} else {
						text = msg.Content
					}
					r <- text

					tok_in, tok_out := res.Usage.PromptTokens, res.Usage.CompletionTokens
					convo.TotalTokens = tok_in + tok_out
					log.Println("Conversation length:", len(convo.Messages))

					if convo.Verbose {
						r <- fmt.Sprintf("ℹ️ Tokens: %d => %d", tok_in, tok_out)
					}
					c.Conversations[tgChatID] = convo

					if len(m) > 0 {
						var ans string
						friend := m[1]
						query := m[2]
						if friend == "Bing" {
							ans, err = c.Bing.SendQuery(query)
						} else if friend == "Wolfram" {
							ans, err = c.Wolfram.SendQuery(query)
						} else {
							continue
						}
						if err != nil {
							return
						}
						ans = fmt.Sprintf("Please summarize the response:\n%s", ans)
						log.Println(ans)

						convo.Messages = append(convo.Messages,
							Message{Role: "user", Content: ans})
						req := c.MakeRequest(convo.Messages)
						for tries := 1; tries < 3; tries++ {
							err = client.Connect(req, "POST", map[string]string{})
							if err == nil {
								break
							}
							time.Sleep(2 * time.Second)
						}
						if err != nil {
							log.Printf("Couldn't send message: %v", err)
							return
						}
						wait = 2
					}
				}
			}
		}
	}()

	return r, err
}

// func (c *Client) AskBing(query string) error {

// }
