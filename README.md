# GPT4-bot

> Chat with the latest OpenAI language model GPT-4. 

Go CLI to fuels a Telegram bot that lets you interact with [GPT4](https://openai.com/product/gpt-4), a large language model trained by OpenAI.

## Installation

Clone this repository and run `go build` in its root directory. 

After you download the file, extract it into a folder and open the `env.example` file with a text editor and fill in your credentials. 

- `TELEGRAM_TOKEN`: Your Telegram Bot token
  - Follow [this guide](https://core.telegram.org/bots/tutorial#obtain-your-bot-token) to create a bot and get the token.
- `OPENAI_KEY`: your OpenAI API key. You will need to be invited by OpenAI to have access to the GPT4 model. 
- `TELEGRAM_ID` (Optional): Your Telegram User ID
  - If you set this, only you will be able to interact with the bot.
  - To get your ID, message `@userinfobot` on Telegram.
  - Multiple IDs can be provided, separated by commas.
- `EDIT_WAIT_SECONDS` (Optional): Amount of seconds to wait between edits
  - This is set to `1` by default, but you can increase if you start getting a lot of `Too Many Requests` errors.
- Save the file, and rename it to `.env`.

> **Note** Make sure you rename the file to _exactly_ `.env`! The program won't work otherwise.

Finally, run `./openai-telegram`.

## Usage of the Telegram bot

Directly send your message and wait for the reply!
In addition, add "SYSTEM:" before your message to set the system information of your GPT-4 bot. 

Commands:

- /start: start the bot
- /help: get help information
- /reset: clear the conversation history

Interact with a plugin:
`!<plugin_name> <input>`

## TODO

### Ideas

- Sending files (text, image, audio, video, etc.)
  - URL
  - Download and send
- Generate LaTeX figure using TikZ
- Write music using the ABC notation
  - Let the bot synthesize the music and send it as a voice message
- Write a post on Twitter and Reddit, starting with "Hello world!"
  - Write a reply under an Elon Musk's tweet

### Plugins

- Python console
- General HTTP request
- Bing Search
- Google Search
- Google Maps
- Google Play
- Gmail
- YouTube
- Stack Overflow
- Wolfram Alpha
- Twitter
- Facebook
- Amazon
- Slack
- Discord
- Notion
- Expedia
- Zapier?
- Speak (text-to-speech)
- FiscalNote
- Yahoo! Finance
- KAYAK?

#### Plugin syntax

- Bot:
  - Input:
    - 🤖 I ask {{interface}}\n\n{{input}}
    - 🤖 I tell {{interface}}\n\n{{input}} (POST request)
  - Output:
    - 🤖 {{interface}} replies\n\n{{output}}
- User:
  - Input:
    - /{{interface}} {{input}}
  - Output (by bot):
    - 🤖 {{interface}} replies\n\n{{output}}

#### Plugin workflow

- bot <=> plugin
  1. 🤖 I ask {{interface}}\n\n{{input}}
  2. 🤖 {{interface}} replies\n\n{{output}}
  3. ... (repeat)
  4. {{final_answer}}
- user <=> plugin
  1. /{{interface}} {{input}}
  2. 🤖 {{interface}} replies\n\n{{output}}

### Save and load conversation history

JSON format
