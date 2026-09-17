package schedulerclient

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{Timeout: 5*time.Second},
	}
}

func (c *Client) NotifyVideoDeleted(ctx context.Context, vid string) error{
	url := c.baseURL + "/video-del-rec/" + vid
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("notify scheduler: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("scheduler returned %d", response.StatusCode)
	}
	return nil
}