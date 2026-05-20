package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Shopify interacts with the Shopify Admin REST API.
// Actions: list_products, get_product, create_product,
//          list_orders, get_order, create_order, update_inventory.
//
// Example:
//
//	nodes.NewShopify("your-store.myshopify.com", "shpat_access_token")
type Shopify struct {
	Store  string // e.g. "your-store.myshopify.com"
	Token  string
	client *http.Client
}

func NewShopify(store, token string) *Shopify {
	return &Shopify{Store: store, Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *Shopify) Name() string { return "shopify" }

func (s *Shopify) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Shopify store. Actions: list_products, get_product, list_orders, get_order, create_product.",
		Params: map[string]any{
			"action":     map[string]any{"type": "string", "desc": "list_products | get_product | list_orders | get_order | create_product"},
			"id":         map[string]any{"type": "string", "desc": "Product or order ID."},
			"title":      map[string]any{"type": "string", "desc": "Product title (create_product)."},
			"price":      map[string]any{"type": "string", "desc": "Product price e.g. '9.99' (create_product)."},
			"limit":      map[string]any{"type": "integer", "desc": "Max results. Default 10."},
		},
	}
}

func (s *Shopify) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := fmt.Sprintf("https://%s/admin/api/2024-01", s.Store)
	action, _ := in["action"].(string)

	switch action {
	case "list_products", "":
		limit := 10
		if v, ok := in["limit"].(float64); ok {
			limit = int(v)
		}
		return s.get(ctx, fmt.Sprintf("%s/products.json?limit=%d", base, limit))

	case "get_product":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("shopify: id required")
		}
		return s.get(ctx, fmt.Sprintf("%s/products/%s.json", base, id))

	case "list_orders":
		limit := 10
		if v, ok := in["limit"].(float64); ok {
			limit = int(v)
		}
		return s.get(ctx, fmt.Sprintf("%s/orders.json?limit=%d&status=any", base, limit))

	case "get_order":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("shopify: id required")
		}
		return s.get(ctx, fmt.Sprintf("%s/orders/%s.json", base, id))

	case "create_product":
		title, _ := in["title"].(string)
		price, _ := in["price"].(string)
		if title == "" {
			return nil, fmt.Errorf("shopify: title required")
		}
		if price == "" {
			price = "0.00"
		}
		body := map[string]any{
			"product": map[string]any{
				"title": title,
				"variants": []any{
					map[string]any{"price": price},
				},
			},
		}
		return s.post(ctx, base+"/products.json", body)

	default:
		return nil, fmt.Errorf("shopify: unknown action %q", action)
	}
}

func (s *Shopify) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Shopify) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Shopify) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("x-shopify-access-token", s.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("shopify: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("shopify: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
