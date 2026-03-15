package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Notifier Telegram’a mesaj gönderir. Token ve chatID boşsa göndermez.
type Notifier struct {
	token  string
	chatID string
	client *http.Client
}

// New Notifier oluşturur. token veya chatID boşsa Send no-op.
func New(token, chatID string) *Notifier {
	return &Notifier{token: token, chatID: chatID, client: &http.Client{}}
}

// Send mesajı Telegram’a gönderir. token/chatID yoksa nil döner.
func (n *Notifier) Send(ctx context.Context, text string) error {
	if n.token == "" || n.chatID == "" {
		return nil
	}
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?chat_id=%s&text=%s",
		n.token, n.chatID, url.QueryEscape(text))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api: %s", resp.Status)
	}
	return nil
}
