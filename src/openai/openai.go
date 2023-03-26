package openai

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/tztsai/openai-telegram/src/bing"
	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/sse"
	"github.com/tztsai/openai-telegram/src/subproc"
	"github.com/tztsai/openai-telegram/src/wolfram"
)

const API_URL = "https://api.openai.com/v1/chat/completions"
const USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"

const MAX_TOKENS = 8192

const QUERY_FAILED = "Query failed. Try a new one."

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
	Python        *subproc.Subproc
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
		Python:        subproc.Init(config.PythonPath, "src/subproc/console.py"),
	}
}

func (c *GPT4) Close() {
	c.Python.Close()
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

func (c *GPT4) AddMessage(chatID int64, message string, role string) Conversation {
	convo := c.GetConversation(chatID)
	convo.Messages = append(convo.Messages, Message{Role: role, Content: message})
	c.Conversations[chatID] = convo
	return convo
}

func (c *GPT4) DelMessage(chatID int64, index int) Conversation {
	convo := c.GetConversation(chatID)
	if index < 0 {
		index = len(convo.Messages) + index
	}
	convo.Messages = append(convo.Messages[:index], convo.Messages[index+1:]...)
	c.Conversations[chatID] = convo
	return convo
}

func (t *Conversation) GetConversationInfo() string {
	return fmt.Sprintf("Length: %d\nTokens: %d\nDuration: %.0f s",
		len(t.Messages), t.TotalTokens, time.Since(t.Time).Seconds())
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

func (c *GPT4) SendRequest(client sse.Client, chatID int64) error {
	req := Request{
		Model:       c.ModelName,
		Messages:    c.GetConversation(chatID).Messages,
		Temperature: c.Temperature,
	}
	err := client.Connect(req, "POST", map[string]string{})
	if err != nil {
		log.Println(err)
	}
	return err
}

func (c *GPT4) SendMessage(message string, tgChatID int64) (chan string, error) {
	var role string
	var err error

	client := c.InitClient()

	// a channel to send the responses back to the main thread
	feed := make(chan string)

	if strings.HasPrefix(message, "/system ") {
		role = "system"
		message = message[8:]
	} else if strings.HasPrefix(message, "/py ") {
		role = "assistant"
		code := message[4:]
		ans, err := c.Python.Send(code)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		go func() { feed <- ans }()
		message = fmt.Sprintf("```python\n>>> %s\n%s```",
			strings.ReplaceAll(code, "\n", "\n... "), ans)
		return nil, nil
	} else {
		role = "user"
	}

	if role != "user" {
		return nil, nil
	}

	convo := c.AddMessage(tgChatID, message, role)
	tokens_thresh := int(math.Round(MAX_TOKENS * 0.9))

	if convo.TotalTokens > tokens_thresh {
		log.Println("Conversation getting too long, deleted earliest responses")
		for i, j := 0, 0; i < len(convo.Messages)-6 && j < 2; i++ {
			if convo.Messages[i].Role == "assistant" {
				convo = c.DelMessage(tgChatID, i)
				j++
			}
		}
	}

	for tries := 1; len(convo.Messages) > 0; tries++ {
		// post HTTP request
		if err := c.SendRequest(client, tgChatID); err != nil {
			exc := strings.Contains(err.Error(), "400 Bad Request") && convo.TotalTokens > 4000
			// if max tokens exceeded, delete earliest messages and try again
			if tries < 3 && exc {
				log.Println(convo.GetConversationInfo())
				convo = c.DelMessage(tgChatID, 0)
				info := "ℹ️ Max tokens exceeded, deleted earliest messages"
				log.Println(info)
				if convo.Verbose {
					go func() { feed <- info }()
				}
			} else {
				return nil, err
			}
		} else {
			break
		}
	}

	var wait = 0
	var plugin string
	var query string
	var ans string

	var py_pat = regexp.MustCompile("🤖\n```python\n([\\s\\S]*)\n```")
	var q_pat = regexp.MustCompile(`🤖 I ask (\w+)\s+([\s\S]*)`)

	go func() {
		defer close(feed)
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
				if err := json.Unmarshal(chunk, &res); err != nil {
					log.Printf("Couldn't unmarshal message response: %v", err)
					return
				}

				if len(res.Choices) > 0 {
					log.Printf("Got response from GPT4:\n%v", res)

					msg := res.Choices[0].Message
					text := msg.Content
					text = strings.ReplaceAll(text, "\r\n", "\n")

					m := py_pat.FindStringSubmatch(text)

					if m != nil {
						plugin = "Python"
						code := m[1]
						ans, err := c.Python.Send(code)
						if err != nil {
							log.Println(err)
							return
						}
						ans = fmt.Sprintf("🤖\n```\n%s\n```", ans)
						log.Println(ans)
						feed <- ans
						c.AddMessage(tgChatID, ans, "assistant")
						c.SendRequest(client, tgChatID)
					}

					m = q_pat.FindStringSubmatch(text)

					if wait == 2 && ans == QUERY_FAILED {
						wait = 0 // try a new query
					}

					if wait == 2 { // summarize the query response
						s := q_pat.Split(text, 2)[0] // avoid a new query in the summary
						text = fmt.Sprintf("🤖 %s answers\n\n%s", plugin, s)

						// replace the last query response with its summary
						c.DelMessage(tgChatID, -1)
						c.AddMessage(tgChatID, text, "assistant")
						wait = 1 // wait for the next message (a final answer or a new query)

						// send a new request with the updated messages
						if c.SendRequest(client, tgChatID) != nil {
							return
						}
					} else {
						convo.Messages = append(convo.Messages, msg)
						wait = 0
					}

					feed <- msg.Content

					tok_in, tok_out := res.Usage.PromptTokens, res.Usage.CompletionTokens
					convo.TotalTokens = tok_in + tok_out
					log.Println(convo.GetConversationInfo())

					if convo.Verbose {
						feed <- fmt.Sprintf("ℹ️ Tokens: %d => %d", tok_in, tok_out)
					}
					c.Conversations[tgChatID] = convo

					if len(m) > 0 {
						plugin = m[1]
						query = m[2]

						if plugin == "Bing" {
							ans, err = c.Bing.Send(query)
						} else if plugin == "Wolfram" {
							ans, err = c.Wolfram.Send(query)
						} else {
							continue
						}
						if err != nil {
							log.Println(err)
							return
						}

						if len(ans) == 0 {
							ans = QUERY_FAILED
							feed <- ans
						} else { // ask GPT to summarize the query response
							ans = fmt.Sprintf("Summarize the response from %s:\n%s", plugin, ans)
						}
						log.Println(ans)

						c.AddMessage(tgChatID, ans, "user")
						if c.SendRequest(client, tgChatID) != nil {
							return
						}

						wait = 2 // wait for 2 more responses (the summary and a new message)
					}
				}
			}
		}
	}()

	return feed, err
}
