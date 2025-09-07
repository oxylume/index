package toncenter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/oxylume/index/pkg/retryhttp"
)

type Client struct {
	endpoint string
	apiKey   string
	client   *retryhttp.Client
}

type getNftResponse struct {
	Items []Nft `json:"nft_items"`
}

func NewClient(endpoint string, apiKey string) *Client {
	return &Client{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		apiKey:   apiKey,
		client:   retryhttp.DefaultClient,
	}
}

func (c *Client) GetNftsByCollection(ctx context.Context, collection string, limit int, offset int) ([]Nft, error) {
	path := fmt.Sprintf("/v3/nft/items?collection_address=%s&limit=%d&offset=%d", collection, limit, offset)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-ok response %s", resp.Status)
	}
	var parsed getNftResponse
	err = json.NewDecoder(resp.Body).Decode(&parsed)
	if err != nil {
		return nil, err
	}
	return parsed.Items, nil
}

func (c *Client) do(ctx context.Context, method string, path string, body io.Reader) (*http.Response, error) {
	url := c.endpoint + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	return c.client.Do(req)
}
