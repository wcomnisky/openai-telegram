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
			cmd             = update.Message.Command()
		)

		if len(envConfig.TelegramID) != 0 && !envConfig.HasTelegramID(updateUserID) {
			log.Printf("User %d is not allowed to use this bot", updateUserID)
			bot.Send(updateChatID, updateMessageID, "You are not authorized to use this bot.")
			continue
		}

		if !update.Message.IsCommand() || cmd == "py" {
			if cmd == "py" {
				updateText = "/py " + strings.TrimSpace(updateText[3:])
			}

			log.Println("Received message:\n", updateText)

			bot.SendTyping(updateChatID)

			feed, err := gpt.SendMessage(updateText, updateChatID)

			if err != nil {
				bot.Send(updateChatID, updateMessageID, fmt.Sprintf("❌ %v", err))
			} else if feed != nil {
				bot.SendAsLiveOutput(updateChatID, updateMessageID, feed)
			}
			continue
		}

		var text string
		switch cmd {
		case "help":
			text = `/reset: clear the bot's memory of this conversation.
/verbose: switch on the verbose mode of the bot ("/verbose off" to switch off).
/ask_friends: allow the bot to ask Bing or Wolfram Alpha before giving an answer.
/system <message>: send a system prompt to the bot.
/temper <value>: set the model temperature (in the range [0.0, 2.0]).
/py <code>: run Python code in the bot's Python interpreter.`
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
		case "load_plugins":
			msg := `You are allowed to interact with external plugins like Python, Bing, and Wolfram Alpha before giving an answer. Make use of the plugins whenever you are unable to answer a question or not sure about your answer.

1. A Python interpreter is available to you for interaction. To send an input to it, the message must follow the format ` + "🤖\\n\\`\\`\\`python\\n<code>\\n\\`\\`\\`" + `. The code block can contain multiple lines like in IPython.
2. You can query Bing and Wolfram by a message starting with ` + "`🤖 I ask (Bing|Wolfram)\\n\\n`" + `. You message should not contain more than one query. Each query should have a simple structure, or it's likely to fail.
3. Ask Bing for real-time web searching and time query.
4. Ask Wolfram for reliable data and scientific computation. Try to make the query structured or in the Wolfram Language.
5. Ensure the accuracy of your final answer, while minimizing your number of queries.`
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
