package openai

import (
	"encoding/json"
	"fmt"
	"log"
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
	Shell         *subproc.Subproc
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
	go c.Python.Close()
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

func (c *GPT4) AddMessage(chatID int64, message string, role string, tokens int) Conversation {
	convo := c.GetConversation(chatID)
	convo.Messages = append(convo.Messages, Message{Role: role, Content: message})
	if tokens > 0 {
		convo.TotalTokens = tokens
	}
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
	return fmt.Sprintf("Length: %d  Tokens: %d  Duration: %.0f s",
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
	err := client.Connect("POST", map[string]string{}, req)
	if err != nil {
		log.Println(err)
	}
	return err
}

func (c *GPT4) SendRequestAvoidTokensExceeded(client sse.Client, chatID int64, retries int) error {
	var err error
	var convo = c.GetConversation(chatID)

	if convo.TotalTokens > MAX_TOKENS-1000 {
		log.Println("Conversation getting too long, deleted earliest responses")
		// delete at most 2 GPT's responses before 6 messages ago
		for i, j := 0, 0; i < len(convo.Messages)-6 && j < 2; i++ {
			if convo.Messages[i].Role == "assistant" {
				convo = c.DelMessage(chatID, i)
				j++
			}
		}
	}

	for tries := 1; len(convo.Messages) > 0; tries++ {
		// send HTTP POST request
		if err = c.SendRequest(client, chatID); err != nil {
			// maybe max tokens exceeded, delete earliest messages and try again
			if tries < retries && strings.Contains(err.Error(), "400 Bad Request") {

				log.Println(convo.GetConversationInfo())

				var i int
				for i = 0; i < 3 && i < len(convo.Messages); i++ {
					if convo.Messages[i].Role != "system" {
						break
					}
				} // try to avoid deleting system messages
				convo = c.DelMessage(chatID, i)

				log.Println("Max tokens exceeded, deleted earliest messages")
			} else {
				return err
			}
		} else {
			return nil
		}
	}
	return err
}

func (c *GPT4) SendSingleMessage(message string) chan string {
	feed := make(chan string)
	go func() {
		defer close(feed)
		feed <- message
	}()
	return feed
}

func (c *GPT4) SendMessage(message string, tgChatID int64) (chan string, error) {
	var role string
	var err error

	client := c.InitClient()

	if strings.HasPrefix(message, "/system ") {
		role = "system"
		message = message[8:]
	} else if strings.HasPrefix(message, "!") {
		gs := regexp.MustCompile(`^!(\w+)\s+([\s\S]*)$`).FindStringSubmatch(message)
		if len(gs) != 3 {
			return nil, fmt.Errorf("invalid command: %s", message)
		}
		plugin := gs[1]
		query := gs[2]

		// directly interact with a plugin
		var ans string
		var err error
		if plugin == "py" {
			ans, err = c.Python.Send(query)
		} else if plugin == "sh" {
			ans, err = c.Shell.Send(query)
		} else if plugin == "bing" {
			ans, err = c.Bing.Send(query)
		} else if plugin == "wolf" {
			ans, err = c.Wolfram.Send(query)
		} else {
			return nil, fmt.Errorf("unknown plugin: %s", plugin)
		}
		if err != nil {
			log.Println(err)
			return nil, err
		}
		return c.SendSingleMessage(ans), nil
	} else {
		role = "user"
	}

	c.AddMessage(tgChatID, message, role, 0)

	if role != "user" {
		return nil, nil
	}

	// send HTTP POST request
	err = c.SendRequestAvoidTokensExceeded(client, tgChatID, 2)
	if err != nil {
		return nil, err
	}

	var wait = 0
	var plugin string
	var query string
	var ans string

	var query_pat = regexp.MustCompile("🤖 I ask (\\w+)\\s+```\\w*\n([\\s\\S]*)```")

	// open the channel to send messages to the Telegram user
	feed := make(chan string)

	go func() {
		defer close(feed)
		for {
			select {
			case chunk, ok := <-client.EventChannel:
				if !ok {
					return
				} else if len(chunk) == 0 {
					if wait > 0 {
						continue // wait for the next response
					} else {
						log.Println("Message feed closed")
						return
					}
				}

				var res MessageResponse
				if err = json.Unmarshal(chunk, &res); err != nil {
					log.Printf("Couldn't unmarshal message response: %v", err)
					feed <- "❌ Failed to decode response from GPT4"
					return
				}

				wait = 0 // received waited response

				if len(res.Choices) > 0 {
					log.Printf("Got response from GPT4:\n%v\n", res)

					msg := res.Choices[0].Message
					text := msg.Content
					text = strings.ReplaceAll(text, "\r\n", "\n")

					// calculate tokens
					tok_in, tok_out := res.Usage.PromptTokens, res.Usage.CompletionTokens
					total_tokens := tok_in + tok_out

					// update chat history
					convo := c.AddMessage(tgChatID, text, "assistant", total_tokens)
					log.Println(convo.GetConversationInfo())

					feed <- text
					if convo.Verbose {
						feed <- fmt.Sprintf("ℹ️ Tokens: %d => %d", tok_in, tok_out)
					}

					// match the query pattern
					match := query_pat.FindStringSubmatch(text)

					if len(match) > 0 {
						plugin = match[1]
						query = match[2]

						log.Printf("Sending query to %s: %s", plugin, query)

						start_time := time.Now()

						if plugin == "Bing" {
							ans, err = c.Bing.Send(query)
						} else if plugin == "Wolfram" {
							ans, err = c.Wolfram.Send(query)
						} else if plugin == "Python" {
							ans, err = c.Python.Send(query)
						} else {
							continue
						}

						if err != nil {
							log.Println(err)
							feed <- "❌ Failed to get response from " + plugin
							return
						}

						if plugin != "Python" && ans == "" {
							ans = QUERY_FAILED
						} else {
							ans = fmt.Sprintf("🤖 %s answers\n\n%s", plugin, ans)
						}

						log.Println(ans)
						feed <- ans

						c.AddMessage(tgChatID, ans, "user", 0)

						time_elapsed := time.Since(start_time)
						t := 1*time.Second - time_elapsed
						if t > 0 {
							time.Sleep(t)
						} // minimum 1 second interval between requests

						err = c.SendRequestAvoidTokensExceeded(client, tgChatID, 2)
						if err != nil {
							log.Println(err)
							return
						}

						wait = 1 // wait for a new response
					}
				}
			}
		}
	}()

	return feed, err
}
