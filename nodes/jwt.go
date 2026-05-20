package nodes

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"orchkit"
)

// JWT signs and verifies JSON Web Tokens using HS256.
// Zero external dependencies.
//
// Example sign:
//
//	nodes.NewJWT("your-secret")
//
// Then call with action="sign", claims={"sub":"user123","role":"admin"}
// Or action="verify", token="eyJ..."
type JWT struct {
	Secret string
}

func NewJWT(secret string) *JWT {
	return &JWT{Secret: secret}
}

func (j *JWT) Name() string { return "jwt" }

func (j *JWT) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Signs or verifies JWT tokens using HS256. Actions: sign, verify.",
		Params: map[string]any{
			"action": map[string]any{"type": "string", "desc": "sign or verify."},
			"claims": map[string]any{"type": "object", "desc": "Claims map to sign (action=sign)."},
			"token":  map[string]any{"type": "string", "desc": "JWT token string to verify (action=verify)."},
			"exp":    map[string]any{"type": "integer", "desc": "Expiry in seconds from now (action=sign). Default 3600."},
		},
	}
}

func (j *JWT) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "sign"
	}

	switch action {
	case "sign":
		claims, _ := in["claims"].(map[string]any)
		if claims == nil {
			claims = map[string]any{}
		}
		exp := 3600
		if v, ok := in["exp"].(float64); ok && v > 0 {
			exp = int(v)
		}
		claims["iat"] = time.Now().Unix()
		claims["exp"] = time.Now().Add(time.Duration(exp) * time.Second).Unix()

		token, err := j.sign(claims)
		if err != nil {
			return nil, fmt.Errorf("jwt: sign: %w", err)
		}
		return orchkit.Output{"token": token, "expires_in": exp}, nil

	case "verify":
		token, _ := in["token"].(string)
		if token == "" {
			return nil, fmt.Errorf("jwt: token is required for verify")
		}
		claims, err := j.verify(token)
		if err != nil {
			return orchkit.Output{"valid": false, "error": err.Error()}, nil
		}
		return orchkit.Output{"valid": true, "claims": claims}, nil

	default:
		return nil, fmt.Errorf("jwt: unknown action %q (use sign or verify)", action)
	}
}

func (j *JWT) sign(claims map[string]any) (string, error) {
	header := base64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64url(claimsJSON)
	sig := j.hmac(header + "." + payload)
	return header + "." + payload + "." + sig, nil
}

func (j *JWT) verify(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}
	expected := j.hmac(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return nil, fmt.Errorf("invalid signature")
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}
	return claims, nil
}

func (j *JWT) hmac(data string) string {
	mac := hmac.New(sha256.New, []byte(j.Secret))
	mac.Write([]byte(data))
	return base64url(mac.Sum(nil))
}

func base64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
