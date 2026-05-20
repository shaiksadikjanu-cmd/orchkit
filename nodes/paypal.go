package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// PayPal interacts with PayPal REST API.
// Actions: create_order, capture_order, get_order, list_payments, refund.
// Uses sandbox by default — set live=true for production.
//
// Example:
//
//	nodes.NewPayPal("client_id", "client_secret", false)
type PayPal struct {
	ClientID     string
	ClientSecret string
	Live         bool
	accessToken  string
	client       *http.Client
}

func NewPayPal(clientID, clientSecret string, live bool) *PayPal {
	return &PayPal{ClientID: clientID, ClientSecret: clientSecret, Live: live, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *PayPal) Name() string { return "paypal" }

func (p *PayPal) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with PayPal REST API. Actions: create_order, capture_order, get_order, refund.",
		Params: map[string]any{
			"action":   map[string]any{"type": "string", "desc": "create_order | capture_order | get_order | refund"},
			"amount":   map[string]any{"type": "string", "desc": "Amount as string e.g. '10.00' (create_order)."},
			"currency": map[string]any{"type": "string", "desc": "Currency code e.g. USD (create_order)."},
			"order_id": map[string]any{"type": "string", "desc": "Order ID (capture_order, get_order)."},
			"capture_id": map[string]any{"type": "string", "desc": "Capture ID (refund)."},
		},
	}
}

func (p *PayPal) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api-m.sandbox.paypal.com"
	if p.Live {
		base = "https://api-m.paypal.com"
	}

	if err := p.ensureToken(ctx, base); err != nil {
		return nil, err
	}

	action, _ := in["action"].(string)

	switch action {
	case "create_order", "":
		amount, _ := in["amount"].(string)
		currency, _ := in["currency"].(string)
		if amount == "" || currency == "" {
			return nil, fmt.Errorf("paypal: amount and currency required")
		}
		body := map[string]any{
			"intent": "CAPTURE",
			"purchase_units": []any{
				map[string]any{
					"amount": map[string]any{
						"currency_code": currency,
						"value":         amount,
					},
				},
			},
		}
		return p.post(ctx, base+"/v2/checkout/orders", body)

	case "capture_order":
		orderID, _ := in["order_id"].(string)
		if orderID == "" {
			return nil, fmt.Errorf("paypal: order_id required")
		}
		return p.post(ctx, fmt.Sprintf("%s/v2/checkout/orders/%s/capture", base, orderID), map[string]any{})

	case "get_order":
		orderID, _ := in["order_id"].(string)
		if orderID == "" {
			return nil, fmt.Errorf("paypal: order_id required")
		}
		return p.get(ctx, base+"/v2/checkout/orders/"+orderID)

	case "refund":
		captureID, _ := in["capture_id"].(string)
		if captureID == "" {
			return nil, fmt.Errorf("paypal: capture_id required for refund")
		}
		return p.post(ctx, fmt.Sprintf("%s/v2/payments/captures/%s/refund", base, captureID), map[string]any{})

	default:
		return nil, fmt.Errorf("paypal: unknown action %q", action)
	}
}

func (p *PayPal) ensureToken(ctx context.Context, base string) error {
	if p.accessToken != "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/oauth2/token",
		strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.ClientID, p.ClientSecret)
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("paypal: auth: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(body, &result)
	p.accessToken = result.AccessToken
	return nil
}

func (p *PayPal) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return p.do(req)
}

func (p *PayPal) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return p.do(req)
}

func (p *PayPal) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+p.accessToken)
	req.Header.Set("content-type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("paypal: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
