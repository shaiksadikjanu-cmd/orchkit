package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// OpenWeather fetches weather data via OpenWeatherMap API.
// Actions: current, forecast, air_quality.
// Free API key at openweathermap.org.
//
// Example:
//
//	nodes.NewOpenWeather("your_api_key")
type OpenWeather struct {
	APIKey string
	client *http.Client
}

func NewOpenWeather(apiKey string) *OpenWeather {
	return &OpenWeather{APIKey: apiKey, client: &http.Client{Timeout: 10 * time.Second}}
}

func (o *OpenWeather) Name() string { return "openweather" }

func (o *OpenWeather) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Fetches weather data from OpenWeatherMap. Actions: current, forecast, air_quality. Free API key available.",
		Params: map[string]any{
			"action": map[string]any{"type": "string", "desc": "current | forecast | air_quality. Default current."},
			"city":   map[string]any{"type": "string", "desc": "City name e.g. 'London' or 'Chennai,IN'."},
			"lat":    map[string]any{"type": "number", "desc": "Latitude (alternative to city)."},
			"lon":    map[string]any{"type": "number", "desc": "Longitude (alternative to city)."},
			"units":  map[string]any{"type": "string", "desc": "metric | imperial | standard. Default metric."},
		},
	}
}

func (o *OpenWeather) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "current"
	}

	units, _ := in["units"].(string)
	if units == "" {
		units = "metric"
	}

	location := ""
	if city, ok := in["city"].(string); ok && city != "" {
		location = fmt.Sprintf("q=%s", city)
	} else if lat, ok := in["lat"].(float64); ok {
		lon, _ := in["lon"].(float64)
		location = fmt.Sprintf("lat=%f&lon=%f", lat, lon)
	} else {
		return nil, fmt.Errorf("openweather: city or lat/lon required")
	}

	var apiURL string
	switch action {
	case "current":
		apiURL = fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?%s&units=%s&appid=%s",
			location, units, o.APIKey)
	case "forecast":
		apiURL = fmt.Sprintf("https://api.openweathermap.org/data/2.5/forecast?%s&units=%s&cnt=8&appid=%s",
			location, units, o.APIKey)
	case "air_quality":
		if lat, ok := in["lat"].(float64); ok {
			lon, _ := in["lon"].(float64)
			apiURL = fmt.Sprintf("https://api.openweathermap.org/data/2.5/air_pollution?lat=%f&lon=%f&appid=%s",
				lat, lon, o.APIKey)
		} else {
			return nil, fmt.Errorf("openweather: lat/lon required for air_quality")
		}
	default:
		return nil, fmt.Errorf("openweather: unknown action %q", action)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("openweather: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openweather: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openweather: api error %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("openweather: parse: %w", err)
	}

	// Extract key fields for easy access.
	out := orchkit.Output{"raw": result}
	if main, ok := result["main"].(map[string]any); ok {
		out["temp"] = main["temp"]
		out["feels_like"] = main["feels_like"]
		out["humidity"] = main["humidity"]
	}
	if weather, ok := result["weather"].([]any); ok && len(weather) > 0 {
		if w, ok := weather[0].(map[string]any); ok {
			out["condition"] = w["main"]
			out["description"] = w["description"]
		}
	}
	if name, ok := result["name"].(string); ok {
		out["city"] = name
	}
	return out, nil
}
