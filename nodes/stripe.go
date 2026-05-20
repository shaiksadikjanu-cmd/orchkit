package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Stripe interacts with the Stripe API.
// Actions: create_customer, create_payment_intent, get_customer,
//          list_charges, create_refund.
//
// Example:
//
//	nodes.NewStripe("sk_test_your_key")
type Stripe struct {
	APIKey string
	client *http.Client
}

func NewStripe(apiKey string) *Stripe {
	return &Stripe{APIKey: apiKey, client: &http.Client{Timeout: 30 * time.Second}}
}

func (s *Stripe) Name() string { return "stripe" }

func (s *Stripe) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Stripe API. Actions: create_customer, create_payment_intent, get_customer, list_charges, create_refund.",
		Params: map[string]any{
			"action":      map[string]any{"type": "string", "desc": "Stripe action to perform."},
			"email":       map[string]any{"type": "string", "desc": "Customer email (create_customer)."},
			"name":        map[string]any{"type": "string", "desc": "Customer name (create_customer)."},
			"amount":      map[string]any{"type": "integer", "desc": "Amount in smallest currency unit e.g. cents (create_payment_intent)."},
			"currency":    map[string]any{"type": "string", "desc": "3-letter currency code e.g. usd (create_payment_intent)."},
			"customer_id": map[string]any{"type": "string", "desc": "Stripe customer ID."},
			"charge_id":   map[string]any{"type": "string", "desc": "Charge ID (create_refund)."},
		},
	}
}

func (s *Stripe) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	switch action {
	case "create_customer":
		params := url.Values{}
		if email, ok := in["email"].(string); ok {
			params.Set("email", email)
		}
		if name, ok := in["name"].(string); ok {
			params.Set("name", name)
		}
		return s.post(ctx, "/v1/customers", params)

	case "create_payment_intent":
		amount, _ := in["amount"].(float64)
		currency, _ := in["currency"].(string)
		if amount <= 0 || currency == "" {
			return nil, fmt.Errorf("stripe: amount and currency required")
		}
		params := url.Values{}
		params.Set("amount", strconv.Itoa(int(amount)))
		params.Set("currency", strings.ToLower(currency))
		if cid, ok := in["customer_id"].(string); ok {
			params.Set("customer", cid)
		}
		return s.post(ctx, "/v1/payment_intents", params)

	case "get_customer":
		cid, _ := in["customer_id"].(string)
		if cid == "" {
			return nil, fmt.Errorf("stripe: customer_id required")
		}
		return s.get(ctx, "/v1/customers/"+cid)

	case "list_charges":
		return s.get(ctx, "/v1/charges?limit=10")

	case "create_refund":
		chargeID, _ := in["charge_id"].(string)
		if chargeID == "" {
			return nil, fmt.Errorf("stripe: charge_id required for refund")
		}
		params := url.Values{}
		params.Set("charge", chargeID)
		return s.post(ctx, "/v1/refunds", params)

	default:
		return nil, fmt.Errorf("stripe: unknown action %q", action)
	}
}

func (s *Stripe) get(ctx context.Context, path string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stripe.com"+path, nil)
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Stripe) post(ctx context.Context, path string, params url.Values) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.stripe.com"+path, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	return s.do(req)
}

func (s *Stripe) do(req *http.Request) (orchkit.Output, error) {
	req.SetBasicAuth(s.APIKey, "")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe: api error %d: %s", resp.StatusCode, body)
	}

	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
