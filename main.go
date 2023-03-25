package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tztsai/openai-telegram/src/config"
	"github.com/tztsai/openai-telegram/src/openai"

	// "github.com/tztsai/openai-telegram/src/session"
	"github.com/tztsai/openai-telegram/src/tgbot"
)

func main() {
	envConfig, err := config.LoadEnvConfig(".env")
	if err != nil {
		log.Fatalf("Couldn't load .env config: %v", err)
	}
	if err := envConfig.ValidateWithDefaults(); err != nil {
		log.Fatalf("Invalid .env config: %v", err)
	}

	gpt := openai.Init(envConfig)
	log.Println("Started GPT-4")

	bot, err := tgbot.New(envConfig.TelegramToken,
		time.Duration(envConfig.EditWaitSeconds*int(time.Second)))
	if err != nil {
		log.Fatalf("Couldn't start Telegram bot: %v", err)
	}

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		bot.Stop()
		os.Exit(0)
	}()

	log.Printf("Started Telegram bot! Message @%s to start.", bot.Username)

	for update := range bot.GetUpdatesChan() {
		if update.Message == nil {
			continue
		}

		var (
			updateText      = update.Message.Text
			updateChatID    = update.Message.Chat.ID
			updateMessageID = update.Message.MessageID
			updateUserID    = update.Message.From.ID
			conversation    = gpt.GetConversation(updateChatID)
		)

		if len(envConfig.TelegramID) != 0 && !envConfig.HasTelegramID(updateUserID) {
			log.Printf("User %d is not allowed to use this bot", updateUserID)
			bot.Send(updateChatID, updateMessageID, "You are not authorized to use this bot.")
			continue
		}

		if !update.Message.IsCommand() {
			log.Println("Received message:\n", updateText)

			bot.SendTyping(updateChatID)

			feed, err := gpt.SendMessage(updateText, updateChatID)
			// if conversation.Verbose {
			// 	for _, info := range infos {
			// 		if len(info) > 2048 {
			// 			info = info[:2048] + "..."
			// 		}
			// 		bot.Send(updateChatID, updateMessageID, "ℹ️ "+info)
			// 	}
			// }
			if err != nil {
				bot.Send(updateChatID, updateMessageID, fmt.Sprintf("❌ %v", err))
			} else if feed != nil {
				bot.SendAsLiveOutput(updateChatID, updateMessageID, feed)
			}
			continue
		}

		var text string
		cmd := update.Message.Command()
		switch cmd {
		case "help":
			text = `/reset: clear the bot's memory of this conversation.
/verbose: switch on the verbose mode of the bot ("/verbose off" to switch off).
/ask_friends: allow the bot to ask Bing or Wolfram Alpha before giving an answer.
/system <message>: send a system prompt to the bot.
/temper <value>: set the model temperature (in the range [0.0, 2.0]).`
		case "start":
			text = "Send a message to start talking with GPT4. Use /help to find available commands."
		case "reset":
			gpt.ResetConversation(updateChatID)
			text = "ℹ️ Started a new conversation. Enjoy!"
		case "system":
			gpt.SendMessage(updateText, updateChatID)
			text = "ℹ️ Added system prompt"
		case "temper":
			t, err := strconv.ParseFloat(strings.TrimSpace(updateText[7:]), 64)
			if err != nil {
				text = "❌ Invalid temperature."
			} else {
				text = fmt.Sprintf("ℹ️ Set temperature to %.2f", t)
				gpt.Temperature = float32(t)
			}
		case "verbose":
			if update.Message.Text == "/verbose off" {
				conversation.Verbose = false
			} else {
				conversation.Verbose = true
			}
			gpt.Conversations[updateChatID] = conversation
			text = fmt.Sprintf("ℹ️ verbose = %s", strconv.FormatBool(conversation.Verbose))
		case "ask_friends":
			msg := `You are allowed to send queries to your friends Python, Bing, and Wolfram Alpha before giving an answer. Whenever you are unable to answer a question or not sure about your answer, you should ask for help.

1. A query must start with ` + "`🤖 I ask (Python|Bing|Wolfram):`" + `. A message may contain at most one query. Each query should have a simple structure, or it's likely to fail. If no query, the message is regarded as your final answer.
2. A Python interpreter is available to you for interaction. After the prefix, the query should be Python code wrapped by ` + "\\`\\`\\`" + `. The state of the interpreter is persistent across queries. The response will be the stdout of the interpreter.
3. Ask Bing for up-to-date web searching and time query.
4. Ask Wolfram for reliable data and scientific computation. Try to make the query structured and precise.
5. Ensure the accuracy of your final answer, while minimizing your number of queries and their lengths.`
			gpt.SendMessage("/system "+msg, updateChatID)
			text = "ℹ️ Added system prompt:\n\n" + msg
		case "chats":
			for _, chatID := range gpt.GetChatIDs() {
				text += fmt.Sprintf("/chat_%d\n", chatID)
			}
		default:
			if strings.HasPrefix(cmd, "chat_") {
				i, err := strconv.Atoi(cmd[5:])
				if err != nil {
					text = "Unknown chat ID."
				} else {
					convo := gpt.Conversations[int64(i)]
					text = convo.GetConversationInfo()
					for _, msg := range convo.Messages {
						log.Println(msg)
					}
				}
			} else {
				text = "ℹ️ Unknown command. Send /help to see a list of commands."
			}
		}

		if _, err := bot.Send(updateChatID, updateMessageID, text); err != nil {
			log.Printf("Error sending message: %v", err)
		}
	}
}
