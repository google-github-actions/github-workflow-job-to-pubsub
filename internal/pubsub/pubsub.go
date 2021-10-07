package pubsub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

// Message represents a message on the PubSub topic/subscription.
type Message struct {
	// Data is the message data.
	Data []byte `json:"data"`

	// Attributes are any message attributes.
	Attributes map[string]string `json:"attributes"`

	// MessageID is assigned by the server.
	MessageID string `json:"messageId"`

	// PublishTime is the server timestamp for when the message was published.
	PublishTime *time.Time `json:"publishTime"`
}

type Client struct {
	httpClient *http.Client
}

// NewClient creates a new authenticated http client using Google's
// Application Default Credentials.
func NewClient() (*Client, error) {
	httpClient, err := google.DefaultClient(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to set up credentials: %w", err)
	}

	return &Client{
		httpClient: httpClient,
	}, nil
}

type Messages struct {
	Messages []*Message `json:"messages"`
}

// Publish pushes new messages onto the provided topic, returning any errors
// that occur.
func (c *Client) Publish(ctx context.Context, topic string, messages []*Message) error {
	pth := fmt.Sprintf("https://pubsub.googleapis.com/v1/%s:publish", topic)

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(&Messages{
		Messages: messages,
	}); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pth, &body)
	if err != nil {
		return fmt.Errorf("failed to build publish request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make publish request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := ioutil.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return fmt.Errorf("bad response from server on publish (%d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// PublishOne pushes a new message onto the provided topic, returning any errors
// that occur.
func (c *Client) PublishOne(ctx context.Context, topic string, message *Message) error {
	return c.Publish(ctx, topic, []*Message{message})
}

// PullAndAck pulls the message and immediately acks it.
func (c *Client) PullAndAck(ctx context.Context, subscription string) (*Message, error) {
	message, err := c.Pull(ctx, subscription)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, nil
	}

	if err := c.Ack(ctx, subscription, message.AckID); err != nil {
		return nil, err
	}

	return message.Message, err
}

type ReceivedMessages struct {
	ReceivedMessages []*ReceivedMessage `json:"receivedMessages"`
}

// ReceivedMessage is a message received back from PubSub.
type ReceivedMessage struct {
	AckID   string   `json:"ackId"`
	Message *Message `json:"message"`
}

// Pull gets a message of the given subscription. Use a timeout on the context
// to cancel the pull. The returned message will be nil if there's nothing on
// the subscription.
func (c *Client) Pull(ctx context.Context, subscription string) (*ReceivedMessage, error) {
	pth := fmt.Sprintf("https://pubsub.googleapis.com/v1/%s:pull", subscription)

	body := strings.NewReader(`{"maxMessages":1}`)
	req, err := http.NewRequestWithContext(ctx, "POST", pth, body)
	if err != nil {
		return nil, fmt.Errorf("failed to build pull request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := ioutil.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return nil, fmt.Errorf("bad response from server on pull (%d): %s", resp.StatusCode, respBody)
	}

	var messages ReceivedMessages
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pull esponse: %w", err)
	}

	if len(messages.ReceivedMessages) == 0 {
		return nil, nil
	}

	return messages.ReceivedMessages[0], nil
}

type AckMessage struct {
	AckIDs []string `json:"ackIds"`
}

func (c *Client) Ack(ctx context.Context, subscription, ackID string) error {
	pth := fmt.Sprintf("https://pubsub.googleapis.com/v1/%s:acknowledge", subscription)

	message := &AckMessage{
		AckIDs: []string{ackID},
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(message); err != nil {
		return fmt.Errorf("failed to encode ack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pth, &body)
	if err != nil {
		return fmt.Errorf("failed to build ack request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make ack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := ioutil.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return fmt.Errorf("bad response from server on ack (%d): %s", resp.StatusCode, respBody)
	}
	return nil
}
