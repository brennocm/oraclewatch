package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// Stdout writes one line per notification. Always available; useful in
// development and as a log of what was sent.
type Stdout struct {
	mu sync.Mutex
	w  io.Writer
}

// NewStdout writes to w.
func NewStdout(w io.Writer) *Stdout { return &Stdout{w: w} }

// Name implements alert.Notifier.
func (*Stdout) Name() string { return "stdout" }

// Send implements alert.Notifier.
func (s *Stdout) Send(_ context.Context, m alert.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.Storm {
		if _, err := fmt.Fprintf(s.w, "ALERT STORM: %d alerts\n", len(m.Notifications)); err != nil {
			return err
		}
		for _, l := range stormLines(m) {
			if _, err := fmt.Fprintln(s.w, "  "+l); err != nil {
				return err
			}
		}
		return nil
	}
	n := m.Notifications[0]
	_, err := fmt.Fprintf(s.w, "%s: %s\n", title(n), n.Result.Summary)
	return err
}

// Discord limits (https://discord.com/developers/docs/resources/message#embed-object-embed-limits).
const (
	discordTitleMax = 256
	discordDescMax  = 4096
	discordFieldMax = 1024
	discordFields   = 25
)

// Colours per status (RGB).
var discordColor = map[checks.Status]int{
	checks.OK: 0x2ECC71, checks.Info: 0x3498DB, checks.Unknown: 0x95A5A6,
	checks.Warning: 0xF1C40F, checks.Critical: 0xE74C3C,
}

// discordMarkdown escapes the characters Discord renders as markdown, so that
// text from untrusted sources (contract strings, RPC errors) cannot inject
// masked links ([text](url)), mentions or formatting into the alert channel.
var discordMarkdown = strings.NewReplacer(
	`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "`", "\\`", "|", `\|`,
	">", `\>`, "[", `\[`, "]", `\]`, "(", `\(`, ")", `\)`, "#", `\#`, "@", "@\u200b",
)

func discordText(s string, n int) string { return truncate(discordMarkdown.Replace(s), n) }

// Discord posts to an incoming webhook.
type Discord struct {
	url string
	p   poster
}

// NewDiscord returns a Discord notifier. The URL is a secret; register it
// with r before use.
func NewDiscord(webhookURL string, client *http.Client, r *redact.Redactor) *Discord {
	return &Discord{url: webhookURL, p: newPoster(client, r)}
}

// Name implements alert.Notifier.
func (*Discord) Name() string { return "discord" }

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type discordPayload struct {
	Username string         `json:"username"`
	Embeds   []discordEmbed `json:"embeds"`
	// No mentions are ever parsed from alert text.
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
}

// Payload builds the webhook body; exported for tests.
func (d *Discord) Payload(m alert.Message) any {
	p := discordPayload{Username: "oraclewatch"}
	p.AllowedMentions.Parse = []string{}
	if m.Storm {
		p.Embeds = []discordEmbed{{
			Title:       truncate(fmt.Sprintf("Alert storm: %d alerts", len(m.Notifications)), discordTitleMax),
			Description: discordText(strings.Join(stormLines(m), "\n"), discordDescMax),
			Color:       discordColor[m.Worst()],
		}}
		return p
	}
	n := m.Notifications[0]
	color := discordColor[n.Result.Status]
	if n.Kind == alert.KindResolved {
		color = discordColor[checks.OK]
	}
	e := discordEmbed{
		Title:       truncate(title(n), discordTitleMax), // built from config names and check IDs only
		Description: discordText(n.Result.Summary, discordDescMax),
		URL:         n.Link,
		Color:       color,
		Timestamp:   n.Result.Observed.UTC().Format(time.RFC3339),
	}
	add := func(name, value string, inline bool) {
		if value == "" || len(e.Fields) >= discordFields {
			return
		}
		e.Fields = append(e.Fields, discordField{Name: truncate(name, discordTitleMax), Value: discordText(value, discordFieldMax), Inline: inline})
	}
	add("Network", n.Result.Network, true)
	add("Feed", n.Result.Feed, true)
	add("Check", n.Result.CheckID, true)
	if !n.Since.IsZero() {
		add("Since", n.Since.UTC().Format(time.RFC3339), true)
	}
	for _, k := range sortedKeys(n.Result.Details) {
		add(k, n.Result.Details[k], true)
	}
	p.Embeds = []discordEmbed{e}
	return p
}

// Send implements alert.Notifier.
func (d *Discord) Send(ctx context.Context, m alert.Message) error {
	return d.p.postJSON(ctx, d.url, d.Payload(m))
}

// telegramMax is the Bot API limit for message text.
const telegramMax = 4096

// TelegramAPI is the Bot API base URL; tests replace it.
const TelegramAPI = "https://api.telegram.org"

// Telegram sends plain-text messages with the Bot API sendMessage method.
// Plain text (no parse_mode) avoids having to escape alert content.
type Telegram struct {
	api    string
	token  string
	chatID string
	p      poster
}

// NewTelegram returns a Telegram notifier. The token is a secret; register it
// with r before use. An empty api selects TelegramAPI.
func NewTelegram(api, token, chatID string, client *http.Client, r *redact.Redactor) *Telegram {
	if api == "" {
		api = TelegramAPI
	}
	return &Telegram{api: strings.TrimRight(api, "/"), token: token, chatID: chatID, p: newPoster(client, r)}
}

// Name implements alert.Notifier.
func (*Telegram) Name() string { return "telegram" }

// Text builds the message text; exported for tests.
func (t *Telegram) Text(m alert.Message) string {
	var b strings.Builder
	if m.Storm {
		fmt.Fprintf(&b, "oraclewatch: alert storm, %d alerts\n", len(m.Notifications))
		for _, l := range stormLines(m) {
			b.WriteString("• " + l + "\n")
		}
		return truncate(b.String(), telegramMax)
	}
	n := m.Notifications[0]
	b.WriteString(title(n) + "\n" + n.Result.Summary + "\n")
	for _, k := range sortedKeys(n.Result.Details) {
		b.WriteString(k + ": " + n.Result.Details[k] + "\n")
	}
	if n.Link != "" {
		b.WriteString(n.Link + "\n")
	}
	return truncate(b.String(), telegramMax)
}

// Send implements alert.Notifier.
func (t *Telegram) Send(ctx context.Context, m alert.Message) error {
	payload := map[string]any{"chat_id": t.chatID, "text": t.Text(m), "disable_web_page_preview": true}
	return t.p.postJSON(ctx, t.api+"/bot"+t.token+"/sendMessage", payload)
}
