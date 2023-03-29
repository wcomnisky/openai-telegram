package sse

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type Client struct {
	URL          string
	EventChannel chan []byte
	Headers      map[string]string
}

func Init(url string) Client {
	return Client{
		URL:          url,
		EventChannel: make(chan []byte),
	}
}

func (c *Client) Connect(method string, params map[string]string, data any) error {
	var body io.Reader
	var err error

	if method == "POST" {
		bs, _ := json.Marshal(&data)
		body = strings.NewReader(string(bs))
	} else {
		body = nil
	}

	req, err := http.NewRequest(method, c.URL, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	if len(params) > 0 {
		q := req.URL.Query()
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	var resp *http.Response

	for i := 0; i < 5; i++ {
		http := &http.Client{}
		resp, err = http.Do(req)
		if err != nil {
			break
		} else if resp.StatusCode == 429 || resp.StatusCode == 400 {
			log.Printf("failed to connect to SSE, retry %d/5", i+1)
			i, _ := rand.Int(rand.Reader, big.NewInt(3000))
			k := i.Int64() + 1000
			time.Sleep(time.Duration(k) * time.Millisecond)
		} else {
			break
		}
	}

	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to connect to SSE: %v", resp.Status)
	}

	go func() {
		defer resp.Body.Close()
		defer close(c.EventChannel)

		for {
			body, _ := ioutil.ReadAll(resp.Body)

			if err != nil {
				log.Println(fmt.Errorf("failed to decode event: %v", err))
				break
			}

			c.EventChannel <- body
		}
	}()

	return nil
}

func (c *Client) FeedForward(handler func(data []byte, feed chan string) (bool, error)) chan string {
	var feed = make(chan string)
	var err error

	go func() {
		defer close(feed)
		var done bool

		for data := range c.EventChannel {
			if len(data) == 0 {
				if done {
					return
				} else {
					continue
				}
			}
			done, err = handler(data, feed)
			if err != nil {
				feed <- fmt.Sprintf("❌ %v", err)
				log.Println(err)
				return
			}
		}
	}()

	return feed
}

func (c *Client) ExtractHtml(maxLen int) chan string {
	return c.FeedForward(func(data []byte, feed chan string) (bool, error) {
		doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(data))

		doc.Find("a").Each(func(i int, el *goquery.Selection) {
			el.ReplaceWithHtml(fmt.Sprintf("[%s](%s)",
				strings.TrimSpace(el.Text()), el.AttrOr("href", "")))
		})
		doc.Find("script,meta,style,button,img,canvas,data,details,embed,footer,form,input,video,source,s,del,audio,iframe").Each(
			func(i int, el *goquery.Selection) {
				el.Remove()
			})

		var s = doc.Find("body")
		var buf bytes.Buffer

		// Slightly optimized vs calling Each: no single selection object created
		var f func(n *html.Node)
		f = func(n *html.Node) {
			if n.Type == html.TextNode {
				buf.WriteString(n.Data + "\t")
			}
			if n.FirstChild != nil {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					f(c)
				}
			}
		}

		for _, n := range s.Nodes {
			f(n)
		}

		content := buf.String()
		if len(content) > maxLen {
			content = content[:maxLen-3] + "..."
		}
		re := regexp.MustCompile(`(\s)(\s)\s*`)
		content = re.ReplaceAllString(content, "$1$2")

		feed <- content
		return true, nil
	})
}

func Fetch(url string) ([]byte, error) {
	client := Init(url)
	err := client.Connect("GET", map[string]string{}, nil)
	if err != nil {
		return nil, err
	}
	return <-client.EventChannel, nil
}
