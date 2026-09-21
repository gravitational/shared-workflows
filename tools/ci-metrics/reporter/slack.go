// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reporter

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gravitational/trace"
	"github.com/slack-go/slack"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

var (
	_ Reporter    = (*Slack)(nil)
	_ Preflighter = (*Slack)(nil)
)

// TypeSlack is the config value selecting this reporter.
const TypeSlack = "slack"

// DefaultSlackTokenEnv is the environment variable read for the bot token
const DefaultSlackTokenEnv = "SLACK_BOT_TOKEN"

// Slack limits on a message.
//
// See https://docs.slack.dev/reference/block-kit/blocks
const (
	// slackMaxBlocks is the number of blocks one message may carry.
	slackMaxBlocks = 50
	// slackMaxTextRunes is the length of a section block's text.
	slackMaxTextRunes = 3000
	// slackMaxFieldRunes is the length of one field in a section block.
	slackMaxFieldRunes = 2000
	// slackMaxFields is the number of fields in a section block.
	slackMaxFields = 10
	// slackMaxHeaderRunes is the length of a header block's text.
	slackMaxHeaderRunes = 150
	// slackMaxMarkdownRunes is the length of every markdown block in one
	// message put together.
	slackMaxMarkdownRunes = 12_000
)

// slackAPI is a subset of [slack.Client] that the reporter requires.
type slackAPI interface {
	AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

var _ slackAPI = (*slack.Client)(nil)

// Slack posts a document to a Slack channel as Block Kit blocks.
type Slack struct {
	name      string
	api       slackAPI
	channel   string
	tokenEnv  string
	username  string
	iconEmoji string
	// maxRows caps the rows printed per table. <=0 means no limit.
	maxRows int
}

// NewSlack creates a new instance of [Slack].
func NewSlack(name string, cfg report.ReporterConfig) (*Slack, error) {
	sc := cfg.Slack

	if sc.Channel == "" {
		return nil, trace.BadParameter("slack.channel required")
	}

	tokenEnv := cmp.Or(sc.TokenEnv, DefaultSlackTokenEnv)
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	if token == "" {
		return nil, trace.BadParameter("Slack bot token required")
	}

	return &Slack{
		name:      name,
		api:       slack.New(token),
		channel:   sc.Channel,
		tokenEnv:  tokenEnv,
		username:  sc.Username,
		iconEmoji: sc.IconEmoji,
		maxRows:   cfg.MaxRows,
	}, nil
}

// Name returns the configured name of this reporter.
func (s *Slack) Name() string {
	return s.name
}

// Close implements [Reporter]. The Slack client holds no resources of its own.
func (s *Slack) Close() error {
	return nil
}

// Preflight checks the token before the caller spends money on queries.
func (s *Slack) Preflight(ctx context.Context) error {
	if _, err := s.api.AuthTestContext(ctx); err != nil {
		return trace.Wrap(err, "slack auth.test failed, check the bot token and make sure the app is added to the channel")
	}
	return nil
}

// Report posts the document to the configured channel.
func (s *Slack) Report(ctx context.Context, doc *report.Document) error {
	if doc == nil {
		return trace.BadParameter("document is required")
	}

	blocks := s.summaryBlocks(doc)
	blocks = append(blocks, metaBlocks(doc)...)

	// An empty ts means no summary was posted, which only happens for a
	// document with nothing in its head at all. The sections then go to the
	// channel rather than to a thread that does not exist.
	ts, err := s.post(ctx, "", capBlocks(blocks), doc)
	if err != nil {
		return trace.Wrap(err, "posting %s to %s", doc.ID, s.channel)
	}

	var errs []error
	for _, section := range tableSections(doc.Sections) {
		for chunk := range slices.Chunk(s.sectionBlocks(section), slackMaxBlocks) {
			if _, err := s.post(ctx, ts, chunk, doc); err != nil {
				errs = append(errs, trace.Wrap(err,
					"posting section %q of %s", section.Heading, doc.ID))
				// The rest of a section that already failed is unlikely to
				// fare better, and would only spam the thread.
				break
			}
		}
	}

	return trace.NewAggregate(errs...)
}

// post sends one message, as a threaded reply when threadTS is set, and
// returns the timestamp that identifies it. Nothing to say means no message
// and so no timestamp.
func (s *Slack) post(ctx context.Context, threadTS string, blocks []slack.Block, doc *report.Document) (string, error) {
	if len(blocks) == 0 {
		return "", nil
	}

	opts := []slack.MsgOption{
		// Blocks carry the message; this is the notification and accessible
		// fallback text.
		slack.MsgOptionText(notificationText(doc), true),
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionDisableLinkUnfurl(),
	}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}
	if s.username != "" {
		opts = append(opts, slack.MsgOptionUsername(s.username))
	}
	if s.iconEmoji != "" {
		opts = append(opts, slack.MsgOptionIconEmoji(s.iconEmoji))
	}

	_, ts, err := s.api.PostMessageContext(ctx, s.channel, opts...)
	return ts, trace.Wrap(err)
}

// tableSections picks out the sections carrying a table, which are the only
// ones Slack renders.
//
// What the others hold is a summary of the tables that follow, and the
// document's headline already names the flakiest test. The same report on
// stdout still carries them.
func tableSections(sections []report.Section) []report.Section {
	out := make([]report.Section, 0, len(sections))
	for _, section := range sections {
		if section.Table != nil {
			out = append(out, section)
		}
	}
	return out
}

// summaryBlocks renders the document header: what the report is, what it ran
// over, and its one-line finding.
func (s *Slack) summaryBlocks(doc *report.Document) []slack.Block {
	var blocks []slack.Block

	if doc.Title != "" {
		blocks = append(blocks,
			slack.NewHeaderBlock(plainText(clip(doc.Title, slackMaxHeaderRunes))))
	}

	if doc.Subtitle != "" {
		blocks = append(blocks, contextBlock(doc.Subtitle))
	}
	if doc.Headline != "" {
		blocks = append(blocks, textBlock(escapeMrkdwn(doc.Headline)))
	}

	return blocks
}

// sectionBlocks renders one section of the document.
func (s *Slack) sectionBlocks(section report.Section) []slack.Block {
	var body []slack.Block

	if section.Text != "" {
		body = append(body, textBlock(escapeMrkdwn(section.Text)))
	}

	body = append(body, metricBlocks(section.Metrics)...)
	body = append(body, s.tableBlocks(section.Table)...)

	if len(body) == 0 {
		return nil
	}

	if section.Heading == "" {
		return body
	}
	return append(
		[]slack.Block{textBlock("*" + escapeMrkdwn(section.Heading) + "*")},
		body...)
}

// metricBlocks renders metrics as two-column fields.
func metricBlocks(metrics []report.Metric) []slack.Block {
	var blocks []slack.Block

	for chunk := range slices.Chunk(metrics, slackMaxFields) {
		fields := make([]*slack.TextBlockObject, 0, len(chunk))
		for _, m := range chunk {
			text := "*" + escapeMrkdwn(m.Name) + "*\n" + escapeMrkdwn(m.Value)
			fields = append(fields, markdown(clipMrkdwn(text, slackMaxFieldRunes)))
		}
		blocks = append(blocks, slack.NewSectionBlock(nil, fields, nil))
	}

	return blocks
}

// tableBlocks renders a table as a markdown block, which Slack lays out as a
// real table. Cells are escaped for markdown rather than mrkdwn, so this is
// the one place in the message that does not go through [escapeMrkdwn].
func (s *Slack) tableBlocks(table *report.Table) []slack.Block {
	md, shown, truncated := markdownTable(table, s.maxRows)
	if md == "" {
		return nil
	}

	blocks := []slack.Block{
		slack.NewMarkdownBlock("", clip(md, slackMaxMarkdownRunes)),
	}

	if truncated {
		blocks = append(blocks, contextBlock(
			fmt.Sprintf("showing %d of %d rows", shown, table.TotalRows)))
	}

	return blocks
}

// metaBlocks renders the document's metadata
func metaBlocks(doc *report.Document) []slack.Block {
	var parts []string

	if !doc.Meta.From.IsZero() {
		parts = append(parts, report.Window(doc.Meta.From, doc.Meta.To))
	}
	if scanned := doc.Meta.DataScannedBytes; scanned > 0 {
		parts = append(parts, fmt.Sprintf("%s scanned across %d query/queries",
			report.Bytes(scanned), len(doc.Meta.QueryExecutionIDs)))
	}
	if !doc.Meta.GeneratedAt.IsZero() {
		parts = append(parts, "generated "+doc.Meta.GeneratedAt.UTC().Format(time.RFC3339))
	}

	if len(parts) == 0 {
		return nil
	}
	return []slack.Block{contextBlock(strings.Join(parts, " | "))}
}

// capBlocks trims a message to Slack's block limit, saying that it did so
// rather than dropping the tail silently.
func capBlocks(blocks []slack.Block) []slack.Block {
	if len(blocks) <= slackMaxBlocks {
		return blocks
	}

	dropped := len(blocks) - (slackMaxBlocks - 1)
	kept := slices.Clone(blocks[:slackMaxBlocks-1])
	return append(kept, contextBlock(
		fmt.Sprintf("truncated, %d more block(s) did not fit in one message", dropped)))
}

// notificationText is the fallback shown in notifications and to screen
// readers, where blocks are not rendered. Slack rejects an empty one so we have
// come up with something
func notificationText(doc *report.Document) string {
	title := cmp.Or(doc.Title, doc.ID, "ci-metrics report")
	if doc.Headline == "" {
		return title
	}
	return title + ": " + doc.Headline
}

// textBlock is a section block of already escaped mrkdwn, clipped to Slack's
// limit.
func textBlock(mrkdwn string) slack.Block {
	return slack.NewSectionBlock(markdown(clipMrkdwn(mrkdwn, slackMaxTextRunes)), nil, nil)
}

// contextBlock is the small grey text under a message.
func contextBlock(text string) slack.Block {
	return slack.NewContextBlock("",
		markdown(clipMrkdwn(escapeMrkdwn(text), slackMaxTextRunes)))
}

// markdown builds a mrkdwn text object.
func markdown(text string) *slack.TextBlockObject {
	return slack.NewTextBlockObject(slack.MarkdownType, text, false, false)
}

// plainText builds a plain text object, with emoji shortcodes rendered.
func plainText(text string) *slack.TextBlockObject {
	return slack.NewTextBlockObject(slack.PlainTextType, text, true, false)
}

// mrkdwnEscaper escapes the three characters Slack reserves in message text.
//
// See https://docs.slack.dev/messaging/formatting-message-text
var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// escapeMrkdwn makes text safe to send as mrkdwn, so that a test name
// containing, say, a generic type parameter is not read as a link.
func escapeMrkdwn(text string) string {
	return mrkdwnEscaper.Replace(text)
}

// clip shortens text to at most limit runes, marking where it was cut. A
// limit of zero or less leaves the text alone.
func clip(text string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit-1]) + "..."
}

// clipMrkdwn is [clip] for text that has already been escaped.
func clipMrkdwn(text string, limit int) string {
	out := clip(text, limit)
	if out == text {
		return out
	}

	// Never leave half an entity such as "&am" behind
	cut := strings.TrimSuffix(out, "...")
	if i := strings.LastIndexByte(cut, '&'); i >= 0 && !strings.ContainsRune(cut[i:], ';') {
		return cut[:i] + "..."
	}

	return out
}
