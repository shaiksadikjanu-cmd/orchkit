package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"orchkit"
	"orchkit/nodes"
)

var registry *orchkit.Registry

func main() {
	registry = buildRegistry()

	addr := ":9091"
	if len(os.Args) > 1 {
		addr = ":" + os.Args[1]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveUI)
	mux.HandleFunc("/api/nodes", apiNodes)
	mux.HandleFunc("/api/run", apiRun)

	fmt.Printf("orchkit UI: http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	// Serve from static/ next to the binary, or relative to source.
	_, file, _, _ := runtime.Caller(0)
	htmlPath := filepath.Join(filepath.Dir(file), "static", "index.html")
	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		// Try relative to cwd
		htmlPath = "cmd/orchkit-ui/static/index.html"
	}
	http.ServeFile(w, r, htmlPath)
}

type NodeInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Params      map[string]any `json:"params"`
	Color       string         `json:"color"`
}

var catMap = map[string]string{
	"http_get": "HTTP/Web", "http_post": "HTTP/Web", "webhook": "HTTP/Web",
	"json_parse": "Data", "json_build": "Data", "csv_read": "Data",
	"xml": "Data", "template": "Data", "markdown": "Data", "rss": "Data",
	"sqlite": "Database", "postgres": "Database", "mysql": "Database",
	"mongodb": "Database", "redis": "Database",
	"slack": "Messaging", "discord": "Messaging", "telegram": "Messaging",
	"twilio": "Messaging", "smtp": "Messaging", "whatsapp": "Messaging",
	"s3": "Cloud", "kafka": "Cloud", "dropbox": "Cloud",
	"jwt": "Auth", "ssh": "Auth",
	"github": "Developer", "gitlab": "Developer", "jira": "Developer",
	"linear": "Developer", "circleci": "Developer",
	"hubspot": "CRM", "salesforce": "CRM", "airtable": "CRM", "pipedrive": "CRM",
	"google_sheets": "Productivity", "gmail": "Productivity",
	"notion": "Productivity", "cron": "Productivity", "zoom": "Productivity",
	"wordpress": "Productivity", "trello": "Productivity",
	"shopify": "Productivity", "mailchimp": "Productivity",
	"llm": "AI/LLM", "llm_groq": "AI/LLM", "llm_gemini": "AI/LLM", "openai": "AI/LLM",
	"twitter": "Social", "reddit": "Social",
	"stripe": "Payments", "paypal": "Payments",
	"zendesk": "Support",
	"asana": "Task", "clickup": "Task", "todoist": "Task",
	"openweather": "Utilities", "spotify": "Utilities",
	"delay": "Utilities", "env": "Utilities", "shell": "Utilities",
	"fs_read": "Utilities", "fs_write": "Utilities",
}

var catColors = map[string]string{
	"HTTP/Web": "#6366f1", "Data": "#8b5cf6", "Database": "#0ea5e9",
	"Messaging": "#22c55e", "Cloud": "#f59e0b", "Auth": "#ef4444",
	"Developer": "#ec4899", "CRM": "#14b8a6", "Productivity": "#f97316",
	"AI/LLM": "#a855f7", "Social": "#06b6d4", "Payments": "#84cc16",
	"Support": "#fb923c", "Task": "#64748b", "Utilities": "#94a3b8",
}

func apiNodes(w http.ResponseWriter, r *http.Request) {
	reg := registry.Build()
	names := registry.Names()
	sort.Strings(names)

	var result []NodeInfo
	for _, name := range names {
		node := reg[name]
		schema := node.Schema()
		cat := catMap[name]
		if cat == "" {
			cat = "Utilities"
		}
		result = append(result, NodeInfo{
			Name:        name,
			Description: schema.Description,
			Category:    cat,
			Params:      schema.Params,
			Color:       catColors[cat],
		})
	}

	w.Header().Set("content-type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type RunRequest struct {
	Name  string `json:"name"`
	Steps []struct {
		ID    string         `json:"id"`
		Node  string         `json:"node"`
		Input map[string]any `json:"input"`
	} `json:"steps"`
}

func apiRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Build YAML.
	var sb strings.Builder
	sb.WriteString("name: " + req.Name + "\nsteps:\n")
	for _, step := range req.Steps {
		sb.WriteString("  - id: " + step.ID + "\n    node: " + step.Node + "\n")
		if len(step.Input) > 0 {
			sb.WriteString("    input:\n")
			for k, v := range step.Input {
				sb.WriteString(fmt.Sprintf("      %s: %q\n", k, fmt.Sprint(v)))
			}
		}
	}
	yaml := sb.String()

	tmp, err := os.CreateTemp("", "orchkit-*.yaml")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(yaml)
	tmp.Close()

	flow, err := orchkit.LoadYAML(tmp.Name())
	if err != nil {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "yaml": yaml})
		return
	}

	reg := registry.Build()
	ctx := context.Background()
	store := orchkit.NewMemStore()

	hooks := &orchkit.Hooks{
		OnStepEnd: func(id string, out orchkit.Output, err error, elapsed time.Duration) {},
	}

	state, runErr := orchkit.RunYAML(ctx, flow, reg, store, orchkit.RunOptions{Hooks: hooks})

	w.Header().Set("content-type", "application/json")
	resp := map[string]any{"state": state, "yaml": yaml}
	if runErr != nil {
		resp["error"] = runErr.Error()
	}
	json.NewEncoder(w).Encode(resp)
}

func buildRegistry() *orchkit.Registry {
	r := orchkit.NewRegistry()
	r.Register("http_get",  func() orchkit.Node { return nodes.NewHTTPGet("") })
	r.Register("http_post", func() orchkit.Node { return nodes.NewHTTPPost("") })
	r.Register("webhook",   func() orchkit.Node { return nodes.NewWebhook(":8080", "/hook", 0) })
	r.Register("json_parse",  func() orchkit.Node { return nodes.NewJSONParse("") })
	r.Register("json_build",  func() orchkit.Node { return nodes.NewJSONBuild() })
	r.Register("csv_read",    func() orchkit.Node { return nodes.NewCSVRead("") })
	r.Register("xml",         func() orchkit.Node { return nodes.NewXML("parse") })
	r.Register("template",    func() orchkit.Node { return nodes.NewTemplate("") })
	r.Register("markdown",    func() orchkit.Node { return nodes.NewMarkdown() })
	r.Register("rss",         func() orchkit.Node { return nodes.NewRSS("") })
	r.Register("sqlite",   func() orchkit.Node { return nodes.NewSQLite("", "") })
	r.Register("postgres", func() orchkit.Node { return nodes.NewPostgres("", "") })
	r.Register("mysql",    func() orchkit.Node { return nodes.NewMySQL("", "") })
	r.Register("mongodb",  func() orchkit.Node { return nodes.NewMongoDB("", "", "") })
	r.Register("redis",    func() orchkit.Node { return nodes.NewRedis("", "") })
	r.Register("slack",    func() orchkit.Node { return nodes.NewSlack(os.Getenv("SLACK_TOKEN")) })
	r.Register("discord",  func() orchkit.Node { return nodes.NewDiscord(os.Getenv("DISCORD_TOKEN")) })
	r.Register("telegram", func() orchkit.Node { return nodes.NewTelegram(os.Getenv("TELEGRAM_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID")) })
	r.Register("twilio",   func() orchkit.Node { return nodes.NewTwilio(os.Getenv("TWILIO_ACCOUNT_SID"), os.Getenv("TWILIO_AUTH_TOKEN"), os.Getenv("TWILIO_FROM")) })
	r.Register("smtp",     func() orchkit.Node { return nodes.NewSMTP(os.Getenv("SMTP_HOST"), 587, os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASS")) })
	r.Register("whatsapp", func() orchkit.Node { return nodes.NewWhatsApp(os.Getenv("WHATSAPP_TOKEN"), os.Getenv("WHATSAPP_PHONE_ID")) })
	r.Register("twitter",  func() orchkit.Node { return nodes.NewTwitter(os.Getenv("TWITTER_BEARER_TOKEN")) })
	r.Register("s3",       func() orchkit.Node { return nodes.NewS3(os.Getenv("AWS_REGION"), os.Getenv("AWS_ACCESS_KEY"), os.Getenv("AWS_SECRET_KEY"), os.Getenv("S3_BUCKET")) })
	r.Register("kafka",    func() orchkit.Node { return nodes.NewKafka([]string{os.Getenv("KAFKA_BROKER")}, "") })
	r.Register("dropbox",  func() orchkit.Node { return nodes.NewDropbox(os.Getenv("DROPBOX_TOKEN")) })
	r.Register("jwt",      func() orchkit.Node { return nodes.NewJWT(os.Getenv("JWT_SECRET")) })
	r.Register("ssh",      func() orchkit.Node { return nodes.NewSSH(os.Getenv("SSH_ADDRESS"), os.Getenv("SSH_PASSWORD"), os.Getenv("SSH_KEY_PATH")) })
	r.Register("github",   func() orchkit.Node { return nodes.NewGitHub(os.Getenv("GITHUB_TOKEN")) })
	r.Register("gitlab",   func() orchkit.Node { return nodes.NewGitLab(os.Getenv("GITLAB_TOKEN"), "") })
	r.Register("jira",     func() orchkit.Node { return nodes.NewJira(os.Getenv("JIRA_DOMAIN"), os.Getenv("JIRA_EMAIL"), os.Getenv("JIRA_TOKEN")) })
	r.Register("linear",   func() orchkit.Node { return nodes.NewLinear(os.Getenv("LINEAR_API_KEY")) })
	r.Register("circleci", func() orchkit.Node { return nodes.NewCircleCI(os.Getenv("CIRCLECI_TOKEN")) })
	r.Register("hubspot",    func() orchkit.Node { return nodes.NewHubSpot(os.Getenv("HUBSPOT_TOKEN")) })
	r.Register("salesforce", func() orchkit.Node { return nodes.NewSalesforce(os.Getenv("SALESFORCE_INSTANCE_URL"), os.Getenv("SALESFORCE_TOKEN")) })
	r.Register("airtable",   func() orchkit.Node { return nodes.NewAirtable(os.Getenv("AIRTABLE_TOKEN"), os.Getenv("AIRTABLE_BASE_ID"), os.Getenv("AIRTABLE_TABLE")) })
	r.Register("pipedrive",  func() orchkit.Node { return nodes.NewPipedrive(os.Getenv("PIPEDRIVE_TOKEN")) })
	r.Register("zendesk",    func() orchkit.Node { return nodes.NewZendesk(os.Getenv("ZENDESK_SUBDOMAIN"), os.Getenv("ZENDESK_EMAIL"), os.Getenv("ZENDESK_TOKEN")) })
	r.Register("stripe",     func() orchkit.Node { return nodes.NewStripe(os.Getenv("STRIPE_API_KEY")) })
	r.Register("paypal",     func() orchkit.Node { return nodes.NewPayPal(os.Getenv("PAYPAL_CLIENT_ID"), os.Getenv("PAYPAL_CLIENT_SECRET"), false) })
	r.Register("reddit",     func() orchkit.Node { return nodes.NewReddit(os.Getenv("REDDIT_CLIENT_ID"), os.Getenv("REDDIT_CLIENT_SECRET"), os.Getenv("REDDIT_USERNAME"), os.Getenv("REDDIT_PASSWORD")) })
	r.Register("google_sheets", func() orchkit.Node { return nodes.NewGoogleSheets(os.Getenv("GOOGLE_TOKEN"), os.Getenv("GOOGLE_SPREADSHEET_ID")) })
	r.Register("gmail",         func() orchkit.Node { return nodes.NewGmail(os.Getenv("GOOGLE_TOKEN")) })
	r.Register("notion",        func() orchkit.Node { return nodes.NewNotion(os.Getenv("NOTION_TOKEN")) })
	r.Register("cron",          func() orchkit.Node { return nodes.NewCron("") })
	r.Register("zoom",          func() orchkit.Node { return nodes.NewZoom(os.Getenv("ZOOM_TOKEN")) })
	r.Register("wordpress",     func() orchkit.Node { return nodes.NewWordPress(os.Getenv("WORDPRESS_URL"), os.Getenv("WORDPRESS_USER"), os.Getenv("WORDPRESS_PASS")) })
	r.Register("trello",        func() orchkit.Node { return nodes.NewTrello(os.Getenv("TRELLO_API_KEY"), os.Getenv("TRELLO_TOKEN")) })
	r.Register("shopify",       func() orchkit.Node { return nodes.NewShopify(os.Getenv("SHOPIFY_STORE"), os.Getenv("SHOPIFY_TOKEN")) })
	r.Register("mailchimp",     func() orchkit.Node { return nodes.NewMailchimp(os.Getenv("MAILCHIMP_API_KEY"), os.Getenv("MAILCHIMP_SERVER")) })
	r.Register("asana",         func() orchkit.Node { return nodes.NewAsana(os.Getenv("ASANA_TOKEN")) })
	r.Register("clickup",       func() orchkit.Node { return nodes.NewClickUp(os.Getenv("CLICKUP_TOKEN")) })
	r.Register("todoist",       func() orchkit.Node { return nodes.NewTodoist(os.Getenv("TODOIST_TOKEN")) })
	r.Register("llm",           func() orchkit.Node { return nodes.NewLLM(os.Getenv("ANTHROPIC_API_KEY"), "") })
	r.Register("llm_groq",      func() orchkit.Node { return nodes.NewGroqLLM(os.Getenv("GROQ_API_KEY"), "") })
	r.Register("llm_gemini",    func() orchkit.Node { return nodes.NewGeminiLLM(os.Getenv("GEMINI_API_KEY"), "") })
	r.Register("openai",        func() orchkit.Node { return nodes.NewOpenAI(os.Getenv("OPENAI_API_KEY"), "") })
	r.Register("openweather",   func() orchkit.Node { return nodes.NewOpenWeather(os.Getenv("OPENWEATHER_API_KEY")) })
	r.Register("spotify",       func() orchkit.Node { return nodes.NewSpotify(os.Getenv("SPOTIFY_TOKEN")) })
	r.Register("delay",         func() orchkit.Node { return nodes.NewDelay(0) })
	r.Register("env",           func() orchkit.Node { return nodes.NewEnv() })
	r.Register("shell",         func() orchkit.Node { return nodes.NewShell("") })
	r.Register("fs_read",       func() orchkit.Node { return nodes.NewFSRead("") })
	r.Register("fs_write",      func() orchkit.Node { return nodes.NewFSWrite("") })
	return r
}
