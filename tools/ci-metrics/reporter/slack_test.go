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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gravitational/trace"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

// fakeSlack is a stand-in for the Slack API.
//
// The reporter is pointed at it over HTTP rather than at a hand-written
// [slackAPI], so that the tests cover the Block Kit JSON that really goes on
// the wire.
type fakeSlack struct {
	mu sync.Mutex
	// posts holds the form of every chat.postMessage received, in order.
	posts []url.Values
	// authError and postError are Slack error codes to fail with, such as
	// "invalid_auth" or "not_in_channel". Empty means success.
	authError string
	postError string
}

func (f *fakeSlack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	respond := func(code string, body map[string]any) {
		if code != "" {
			body = map[string]any{"ok": false, "error": code}
		}
		_ = json.NewEncoder(w).Encode(body)
	}

	switch r.URL.Path {
	case "/auth.test":
		respond(f.authError, map[string]any{
			"ok": true, "team": "gravitational", "user": "ci-metrics",
			"team_id": "T1", "user_id": "U1", "bot_id": "B1",
		})
	case "/chat.postMessage":
		f.posts = append(f.posts, r.Form)
		respond(f.postError, map[string]any{
			"ok":      true,
			"channel": r.Form.Get("channel"),
			// A distinct ts per message is what lets a test tell the summary
			// message apart from its thread replies.
			"ts": fmt.Sprintf("1700000000.%06d", len(f.posts)),
		})
	default:
		http.NotFound(w, r)
	}
}

// messages returns the posts received so far.
func (f *fakeSlack) messages() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.posts...)
}

// failAuth makes auth.test fail with a Slack error code.
func (f *fakeSlack) failAuth(code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authError = code
}

// failPosts makes chat.postMessage fail with a Slack error code.
func (f *fakeSlack) failPosts(code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.postError = code
}

// newTestSlack builds a reporter talking to a fake Slack.
func newTestSlack(t *testing.T, cfg report.ReporterConfig) (*Slack, *fakeSlack) {
	t.Helper()

	fake := &fakeSlack{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	t.Setenv(DefaultSlackTokenEnv, "xoxb-not-a-real-token")

	cfg.Type = TypeSlack
	if cfg.Slack.Channel == "" {
		cfg.Slack.Channel = "C0123456789"
	}

	r, err := NewSlack("slack", cfg)
	require.NoError(t, err)

	// slack-go builds method URLs by appending to the endpoint, so the
	// trailing slash matters.
	r.api = slack.New("xoxb-not-a-real-token", slack.OptionAPIURL(srv.URL+"/"))

	return r, fake
}

// postedText flattens every text object of a posted message, which is roughly
// what a reader of that message sees.
func postedText(t *testing.T, form url.Values) string {
	t.Helper()

	var b strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if s, ok := val.(string); ok && k == "text" {
					b.WriteString(s)
					b.WriteByte('\n')
					continue
				}
				walk(val)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}

	walk(postedBlocks(t, form))
	return b.String()
}

// postedBlocks decodes the blocks of a posted message.
func postedBlocks(t *testing.T, form url.Values) []any {
	t.Helper()

	var blocks []any
	require.NoError(t, json.Unmarshal([]byte(form.Get("blocks")), &blocks))
	return blocks
}

// twoTableDocument is a report whose detail runs past the first section.
func twoTableDocument() *report.Document {
	doc := testDocument()
	doc.Sections = append(doc.Sections, report.Section{
		Heading: "2026-09-01",
		Table: &report.Table{
			Columns:   []report.Column{{Name: "TEST", Align: report.AlignLeft}},
			Rows:      []report.Row{{{Text: "TestFromTheDayBefore"}}},
			TotalRows: 1,
		},
	})
	return doc
}

func TestSlackPostsAOneTableReportAsOneMessage(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{})

	require.NoError(t, r.Report(t.Context(), testDocument()))

	posts := fake.messages()
	require.Len(t, posts, 1, "one table needs no thread")

	post := posts[0]
	assert.Empty(t, post.Get("thread_ts"))
	assert.Equal(t, "C0123456789", post.Get("channel"))

	// Header, then the table that is the point of the report, then provenance.
	text := postedText(t, post)
	assert.Contains(t, text, "Top flaky tests")
	assert.Contains(t, text, "*2026-09-02*", "the table's own heading, not the subtitle")
	assert.Contains(t, text, "TestWithALongName")
	assert.Contains(t, text, "| ---: |", "tables are rendered as markdown")
	assert.Contains(t, text, "2.0 KiB scanned across 1 query/queries")

	// The summary section, the headline and every note are still dropped.
	assert.NotContains(t, text, "2 flaky test(s) over 1 day(s)",
		"the headline is the notification text, not a block")
	assert.NotContains(t, text, "Worst")
	assert.NotContains(t, text, "Excludes tests with fewer than 10 executions.")
}

func TestSlackThreadsSectionsPastTheFirst(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{})

	require.NoError(t, r.Report(t.Context(), twoTableDocument()))

	posts := fake.messages()
	require.Len(t, posts, 2, "the second table belongs in the thread")

	lead, reply := posts[0], posts[1]

	assert.Empty(t, lead.Get("thread_ts"), "the lead message starts the thread")

	// The most relevant section is the one read without opening the thread.
	leadText := postedText(t, lead)
	assert.Contains(t, leadText, "*2026-09-02*", "the table's own heading, not the subtitle")
	assert.Contains(t, leadText, "TestWithALongName")
	assert.NotContains(t, leadText, "TestFromTheDayBefore")

	// A reply carries the timestamp of the message it answers, which the fake
	// numbered in order of receipt.
	assert.Equal(t, "1700000000.000001", reply.Get("thread_ts"))
	assert.Equal(t, "C0123456789", reply.Get("channel"))

	replyText := postedText(t, reply)
	assert.Contains(t, replyText, "*2026-09-01*")
	assert.Contains(t, replyText, "TestFromTheDayBefore")
}

func TestSlackSendsFallbackTextForNotifications(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{})

	require.NoError(t, r.Report(t.Context(), testDocument()))

	posts := fake.messages()
	require.NotEmpty(t, posts)
	assert.Equal(t,
		"Top flaky tests: 2 flaky test(s) over 1 day(s)",
		posts[0].Get("text"))
	assert.Equal(t, "false", posts[0].Get("unfurl_links"))
}

func TestSlackAppliesTheCustomisedIdentity(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{
		Slack: report.SlackConfig{
			Username:  "ci-metrics",
			IconEmoji: ":chart_with_upwards_trend:",
		},
	})

	require.NoError(t, r.Report(t.Context(), testDocument()))

	posts := fake.messages()
	require.NotEmpty(t, posts)
	assert.Equal(t, "ci-metrics", posts[0].Get("username"))
	assert.Equal(t, ":chart_with_upwards_trend:", posts[0].Get("icon_emoji"))
}

func TestSlackNotesTruncatedTables(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{MaxRows: 1})

	require.NoError(t, r.Report(t.Context(), testDocument()))

	posts := fake.messages()
	require.Len(t, posts, 1)

	text := postedText(t, posts[0])
	assert.Contains(t, text, "TestA")
	assert.NotContains(t, text, "TestWithALongName")
	assert.Contains(t, text, "showing 1 of 2 rows")
}

func TestSlackReportsEveryFailedSection(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{})
	fake.failPosts("not_in_channel")

	err := r.Report(t.Context(), testDocument())
	require.Error(t, err)
	assert.ErrorContains(t, err, "not_in_channel")
	assert.ErrorContains(t, err, "flaky")

	// The lead message failing means there is no thread to reply to, so
	// nothing further is attempted.
	assert.Len(t, fake.messages(), 1)
}

func TestSlackHandlesASparseDocument(t *testing.T) {
	t.Run("no title means no header block", func(t *testing.T) {
		r, fake := newTestSlack(t, report.ReporterConfig{})

		// A text object may not be empty, so a header here would be a 400
		// from Slack rather than a blank line. The subtitle is what keeps the
		// message alive, since the headline is no longer a block.
		require.NoError(t, r.Report(t.Context(), &report.Document{
			ID:       "flaky",
			Subtitle: "db.testcases_v2_parquet",
			Headline: "nothing to report",
		}))

		posts := fake.messages()
		require.Len(t, posts, 1)

		for _, block := range postedBlocks(t, posts[0]) {
			fields, ok := block.(map[string]any)
			require.True(t, ok, "block is not an object: %v", block)
			assert.NotEqual(t, "header", fields["type"])
		}
		assert.Equal(t, "flaky: nothing to report", posts[0].Get("text"))
	})

	t.Run("a document with nothing in it says nothing", func(t *testing.T) {
		r, fake := newTestSlack(t, report.ReporterConfig{})

		require.NoError(t, r.Report(t.Context(), &report.Document{}))
		assert.Empty(t, fake.messages())
	})
}

func TestSlackRejectsANilDocument(t *testing.T) {
	r, _ := newTestSlack(t, report.ReporterConfig{})

	err := r.Report(t.Context(), nil)
	assert.True(t, trace.IsBadParameter(err), "got %v", err)
}

func TestSlackPreflightChecksTheToken(t *testing.T) {
	t.Run("a working token passes", func(t *testing.T) {
		r, _ := newTestSlack(t, report.ReporterConfig{})
		assert.NoError(t, r.Preflight(t.Context()))
	})

	t.Run("a rejected token names the variable to fix", func(t *testing.T) {
		r, fake := newTestSlack(t, report.ReporterConfig{})
		fake.failAuth("invalid_auth")

		err := r.Preflight(t.Context())
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid_auth")
		assert.ErrorContains(t, err, "check the bot token")
	})
}

func TestNewSlackValidatesItsConfig(t *testing.T) {
	t.Run("the channel is required", func(t *testing.T) {
		t.Setenv(DefaultSlackTokenEnv, "xoxb-not-a-real-token")

		_, err := NewSlack("slack", report.ReporterConfig{Type: TypeSlack})
		assert.True(t, trace.IsBadParameter(err), "got %v", err)
	})

	t.Run("the registry builds one", func(t *testing.T) {
		t.Setenv(DefaultSlackTokenEnv, "xoxb-not-a-real-token")

		r, err := New("ci-health", report.ReporterConfig{
			Type:  TypeSlack,
			Slack: report.SlackConfig{Channel: "C0123456789"},
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, "ci-health", r.Name())
		assert.NoError(t, r.Close())
	})
}

func TestEscapeMrkdwn(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		"TestFoo&lt;T&gt; &amp; friends",
		escapeMrkdwn("TestFoo<T> & friends"))
	assert.Equal(t, "nothing to do", escapeMrkdwn("nothing to do"))
}

func TestClip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{name: "under the limit is untouched", text: "short", limit: 10, want: "short"},
		{name: "exactly the limit is untouched", text: "12345", limit: 5, want: "12345"},
		{name: "over the limit is marked", text: "123456", limit: 5, want: "1234..."},
		{name: "no limit is untouched", text: "123456", limit: 0, want: "123456"},
		{name: "runes are not split", text: "ααααα", limit: 3, want: "αα..."},
		{
			name:  "an ampersand in plain text is kept",
			text:  "fast & loose",
			limit: 8,
			want:  "fast & ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, clip(tt.text, tt.limit))
		})
	}
}

func TestClipMrkdwnKeepsEntitiesWhole(t *testing.T) {
	t.Parallel()

	// Cutting "ab&amp;cd" at five runes would leave "ab&a", which Slack
	// renders literally, so the fragment goes too.
	assert.Equal(t, "ab...", clipMrkdwn("ab&amp;cd", 5))

	// A complete entity before the cut survives.
	assert.Equal(t, "&amp;x...", clipMrkdwn("&amp;xyz", 7))

	assert.Equal(t, "short", clipMrkdwn("short", 10))
}

func TestSlackPostsOnlyTablesAndTheHeader(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{})

	require.NoError(t, r.Report(t.Context(), &report.Document{
		ID:       "flaky_rollup",
		Title:    "Top flaky tests",
		Headline: "1 flaky test(s)",
		Sections: []report.Section{
			{
				Heading: "Summary",
				Metrics: []report.Metric{{Name: "Flaky tests", Value: "1"}},
				Notes:   []string{"Score is 4*p*(1-p)*execs/(execs+20)."},
			},
			{
				Heading: "Window rollup",
				Table: &report.Table{
					Columns:   []report.Column{{Name: "TEST"}},
					Rows:      []report.Row{{{Text: "TestA"}}},
					TotalRows: 1,
				},
				Notes: []string{"At the query's cap of 20; there may be more."},
			},
		},
	}))

	posts := fake.messages()
	require.Len(t, posts, 1)

	text := postedText(t, posts[0])

	// The summary section and the headline go; notes go whether they explain
	// the score or qualify the table.
	assert.NotContains(t, text, "1 flaky test(s)")
	assert.NotContains(t, text, "Summary")
	assert.NotContains(t, text, "Flaky tests")
	assert.NotContains(t, text, "Score is")
	assert.NotContains(t, text, "At the query's cap of 20")

	// The table and its heading stay.
	assert.Contains(t, text, "Window rollup")
	assert.Contains(t, text, "TestA")
}

func TestSlackPostsNothingForAReportWithNoTables(t *testing.T) {
	r, fake := newTestSlack(t, report.ReporterConfig{})

	// The shape of an empty report: a headline, and a section carrying only
	// notes. It is the headline that has to carry the message.
	require.NoError(t, r.Report(t.Context(), &report.Document{
		ID:       "flaky_rollup",
		Title:    "Top flaky tests",
		Headline: "No flaky tests found in the window.",
		Sections: []report.Section{{
			Heading: "Summary",
			Notes:   []string{"No test both passed and failed."},
		}},
	}))

	posts := fake.messages()
	require.Len(t, posts, 1)

	// Only the header survives in the blocks; the headline is the
	// notification text, which is where an empty report still reads.
	text := postedText(t, posts[0])
	assert.Contains(t, text, "Top flaky tests")
	assert.NotContains(t, text, "No flaky tests found in the window.")
	assert.NotContains(t, text, "Summary")
	assert.NotContains(t, text, "No test both passed and failed.")

	assert.Equal(t,
		"Top flaky tests: No flaky tests found in the window.",
		posts[0].Get("text"))
}

func TestMarkdownTable(t *testing.T) {
	t.Parallel()

	table := &report.Table{
		Columns: []report.Column{
			{Name: "TEST", Align: report.AlignLeft},
			{Name: "FAILS", Align: report.AlignRight},
		},
		Rows: []report.Row{
			{{Text: "TestA_Sub[int]"}, {Text: "3"}},
			{{Text: "TestB"}, {Text: "1"}},
		},
		TotalRows: 2,
	}

	t.Run("alignment comes from the columns", func(t *testing.T) {
		t.Parallel()

		md, shown, truncated := markdownTable(table, 0)
		assert.Equal(t, 2, shown)
		assert.False(t, truncated)
		assert.Equal(t, strings.Join([]string{
			"| TEST | FAILS |",
			"| :--- | ---: |",
			`| TestA\_Sub\[int\] | 3 |`,
			"| TestB | 1 |",
		}, "\n"), md)
	})

	t.Run("rows past the cap are dropped", func(t *testing.T) {
		t.Parallel()

		md, shown, truncated := markdownTable(table, 1)
		assert.Equal(t, 1, shown)
		assert.True(t, truncated)
		assert.NotContains(t, md, "TestB")
	})

	t.Run("a short row renders as empty cells", func(t *testing.T) {
		t.Parallel()

		md, _, _ := markdownTable(&report.Table{
			Columns: table.Columns,
			Rows:    []report.Row{{{Text: "TestA"}}},
		}, 0)
		assert.Contains(t, md, "| TestA |  |")
	})

	t.Run("a pipe cannot end a cell early", func(t *testing.T) {
		t.Parallel()

		md, _, _ := markdownTable(&report.Table{
			Columns: []report.Column{{Name: "TEST"}},
			Rows:    []report.Row{{{Text: "a|b"}}},
		}, 0)
		assert.Contains(t, md, `| a\|b |`)
	})

	t.Run("no columns, no table", func(t *testing.T) {
		t.Parallel()

		md, shown, truncated := markdownTable(&report.Table{}, 0)
		assert.Empty(t, md)
		assert.Zero(t, shown)
		assert.False(t, truncated)

		md, _, _ = markdownTable(nil, 0)
		assert.Empty(t, md)
	})
}

func TestCapBlocksMarksWhatItDropped(t *testing.T) {
	t.Parallel()

	var blocks []slack.Block
	for range slackMaxBlocks + 10 {
		blocks = append(blocks, slack.NewDividerBlock())
	}

	capped := capBlocks(blocks)
	require.Len(t, capped, slackMaxBlocks)
	assert.Equal(t, slack.MBTContext, capped[len(capped)-1].BlockType())

	assert.Len(t, capBlocks(blocks[:slackMaxBlocks]), slackMaxBlocks,
		"a message at the limit is left alone")
}

func TestTableSectionsKeepsOnlyTablesInOrder(t *testing.T) {
	t.Parallel()

	sections := []report.Section{
		{Heading: "Summary"},
		{Heading: "Day one", Table: &report.Table{}},
		{Heading: "Day two", Table: &report.Table{}},
	}

	kept := tableSections(sections)
	require.Len(t, kept, 2)
	assert.Equal(t, "Day one", kept[0].Heading)
	assert.Equal(t, "Day two", kept[1].Heading)

	t.Run("a document of nothing but summary keeps nothing", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, tableSections(sections[:1]))
		assert.Empty(t, tableSections(nil))
	})
}
