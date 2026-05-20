package main

import (
	"os"

	"orchkit"
	"orchkit/nodes"
)

func buildRegistry() *orchkit.Registry {
	r := orchkit.NewRegistry()

	// HTTP/Web
	r.Register("http_get",  func() orchkit.Node { return nodes.NewHTTPGet("") })
	r.Register("http_post", func() orchkit.Node { return nodes.NewHTTPPost("") })
	r.Register("webhook",   func() orchkit.Node { return nodes.NewWebhook(":8080", "/hook", 0) })

	// Data
	r.Register("json_parse",  func() orchkit.Node { return nodes.NewJSONParse("") })
	r.Register("json_build",  func() orchkit.Node { return nodes.NewJSONBuild() })
	r.Register("csv_read",    func() orchkit.Node { return nodes.NewCSVRead("") })
	r.Register("xml",         func() orchkit.Node { return nodes.NewXML("parse") })
	r.Register("template",    func() orchkit.Node { return nodes.NewTemplate("") })
	r.Register("markdown",    func() orchkit.Node { return nodes.NewMarkdown() })
	r.Register("rss",         func() orchkit.Node { return nodes.NewRSS("") })

	// Database
	r.Register("sqlite",   func() orchkit.Node { return nodes.NewSQLite("", "") })
	r.Register("postgres", func() orchkit.Node { return nodes.NewPostgres("", "") })
	r.Register("mysql",    func() orchkit.Node { return nodes.NewMySQL("", "") })
	r.Register("mongodb",  func() orchkit.Node { return nodes.NewMongoDB("", "", "") })
	r.Register("redis",    func() orchkit.Node { return nodes.NewRedis("", "") })

	// Messaging — credentials from environment
	r.Register("slack",    func() orchkit.Node { return nodes.NewSlack(os.Getenv("SLACK_TOKEN")) })
	r.Register("discord",  func() orchkit.Node { return nodes.NewDiscord(os.Getenv("DISCORD_TOKEN")) })
	r.Register("telegram", func() orchkit.Node {
		return nodes.NewTelegram(os.Getenv("TELEGRAM_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID"))
	})
	r.Register("twilio", func() orchkit.Node {
		return nodes.NewTwilio(os.Getenv("TWILIO_ACCOUNT_SID"), os.Getenv("TWILIO_AUTH_TOKEN"), os.Getenv("TWILIO_FROM"))
	})
	r.Register("smtp", func() orchkit.Node {
		return nodes.NewSMTP(os.Getenv("SMTP_HOST"), 587, os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASS"))
	})

	// Cloud
	r.Register("s3", func() orchkit.Node {
		return nodes.NewS3(os.Getenv("AWS_REGION"), os.Getenv("AWS_ACCESS_KEY"), os.Getenv("AWS_SECRET_KEY"), os.Getenv("S3_BUCKET"))
	})
	r.Register("kafka", func() orchkit.Node { return nodes.NewKafka([]string{os.Getenv("KAFKA_BROKER")}, "") })

	// Auth
	r.Register("jwt", func() orchkit.Node { return nodes.NewJWT(os.Getenv("JWT_SECRET")) })
	r.Register("ssh", func() orchkit.Node {
		return nodes.NewSSH(os.Getenv("SSH_ADDRESS"), os.Getenv("SSH_PASSWORD"), os.Getenv("SSH_KEY_PATH"))
	})

	// Developer
	r.Register("github", func() orchkit.Node { return nodes.NewGitHub(os.Getenv("GITHUB_TOKEN")) })
	r.Register("jira", func() orchkit.Node {
		return nodes.NewJira(os.Getenv("JIRA_DOMAIN"), os.Getenv("JIRA_EMAIL"), os.Getenv("JIRA_TOKEN"))
	})
	r.Register("linear", func() orchkit.Node { return nodes.NewLinear(os.Getenv("LINEAR_API_KEY")) })

	// CRM
	r.Register("hubspot",    func() orchkit.Node { return nodes.NewHubSpot(os.Getenv("HUBSPOT_TOKEN")) })
	r.Register("salesforce", func() orchkit.Node {
		return nodes.NewSalesforce(os.Getenv("SALESFORCE_INSTANCE_URL"), os.Getenv("SALESFORCE_TOKEN"))
	})
	r.Register("airtable", func() orchkit.Node {
		return nodes.NewAirtable(os.Getenv("AIRTABLE_TOKEN"), os.Getenv("AIRTABLE_BASE_ID"), os.Getenv("AIRTABLE_TABLE"))
	})

	// Productivity
	r.Register("google_sheets", func() orchkit.Node {
		return nodes.NewGoogleSheets(os.Getenv("GOOGLE_TOKEN"), os.Getenv("GOOGLE_SPREADSHEET_ID"))
	})
	r.Register("gmail",  func() orchkit.Node { return nodes.NewGmail(os.Getenv("GOOGLE_TOKEN")) })
	r.Register("notion", func() orchkit.Node { return nodes.NewNotion(os.Getenv("NOTION_TOKEN")) })
	r.Register("cron",   func() orchkit.Node { return nodes.NewCron("") })

	// AI/LLM — credentials from environment
	r.Register("llm",        func() orchkit.Node { return nodes.NewLLM(os.Getenv("ANTHROPIC_API_KEY"), "") })
	r.Register("llm_groq",   func() orchkit.Node { return nodes.NewGroqLLM(os.Getenv("GROQ_API_KEY"), "") })
	r.Register("llm_gemini", func() orchkit.Node { return nodes.NewGeminiLLM(os.Getenv("GEMINI_API_KEY"), "") })
	r.Register("openai",     func() orchkit.Node { return nodes.NewOpenAI(os.Getenv("OPENAI_API_KEY"), "") })

	// Commerce & Productivity
	r.Register("whatsapp",  func() orchkit.Node { return nodes.NewWhatsApp(os.Getenv("WHATSAPP_TOKEN"), os.Getenv("WHATSAPP_PHONE_ID")) })
	r.Register("twitter",   func() orchkit.Node { return nodes.NewTwitter(os.Getenv("TWITTER_BEARER_TOKEN")) })
	r.Register("shopify",   func() orchkit.Node { return nodes.NewShopify(os.Getenv("SHOPIFY_STORE"), os.Getenv("SHOPIFY_TOKEN")) })
	r.Register("mailchimp", func() orchkit.Node { return nodes.NewMailchimp(os.Getenv("MAILCHIMP_API_KEY"), os.Getenv("MAILCHIMP_SERVER")) })
	r.Register("zoom",      func() orchkit.Node { return nodes.NewZoom(os.Getenv("ZOOM_TOKEN")) })
	r.Register("wordpress", func() orchkit.Node { return nodes.NewWordPress(os.Getenv("WORDPRESS_URL"), os.Getenv("WORDPRESS_USER"), os.Getenv("WORDPRESS_PASS")) })
	r.Register("trello",    func() orchkit.Node { return nodes.NewTrello(os.Getenv("TRELLO_API_KEY"), os.Getenv("TRELLO_TOKEN")) })

	// Utilities
	r.Register("delay",    func() orchkit.Node { return nodes.NewDelay(0) })
	r.Register("env",      func() orchkit.Node { return nodes.NewEnv() })
	r.Register("shell",    func() orchkit.Node { return nodes.NewShell("") })
	r.Register("fs_read",  func() orchkit.Node { return nodes.NewFSRead("") })
	r.Register("fs_write", func() orchkit.Node { return nodes.NewFSWrite("") })

	return r
}

func init() {} // ensure file is valid
