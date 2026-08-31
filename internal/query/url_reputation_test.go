package query

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/projection"
	"github.com/generalbusiness-ai/tailapps/tailapps"
)

func TestURLReputationDueAndExclusionQueries(t *testing.T) {
	compiled, err := tailapps.Load("url-reputation")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "url-reputation.sqlite")
	materialized, err := projection.Create(context.Background(), path, compiled, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer materialized.Close()

	position := int64(0)
	observe := func(url, host string) {
		t.Helper()
		position++
		item := delivery(position, "tailapp.url.observed", map[string]any{
			"tailapp.url.observed_full": url,
			"tailapp.url.host":          host,
			"conversation.id":           fmt.Sprintf("session-%d", position),
			"tool_name":                 "web_fetch",
		})
		if _, err := materialized.Process(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	verdict := func(url, provider, validity string) {
		t.Helper()
		position++
		item := delivery(position, "tailapp.url.reputation", map[string]any{
			"tailapp.url.observed_full":         url,
			"tailapp.url.checked_full":          url,
			"tailapp.url.provider":              provider,
			"tailapp.url.verdict":               "clean",
			"tailapp.url.checked_unix_nano":     "1788192000000000000",
			"tailapp.url.valid_until_unix_nano": validity,
		})
		if _, err := materialized.Process(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}

	for _, item := range []struct{ url, host string }{
		{"https://due.example/no-verdict", "due.example"},
		{"https://fresh.example/path", "fresh.example"},
		{"https://expired.example/path", "expired.example"},
		{"https://other.example/path", "other.example"},
		{"https://excluded.example/path", "excluded.example"},
	} {
		observe(item.url, item.host)
	}
	verdict("https://fresh.example/path", "safe-browsing-v5", "1788278400000000000")
	verdict("https://expired.example/path", "safe-browsing-v5", "1788192000000000000")
	verdict("https://other.example/path", "web-risk", "1788278400000000000")
	position++
	if _, err := materialized.Process(context.Background(), delivery(position, "tailapp.url.exclusion", map[string]any{
		"tailapp.url.exclusion.id":      "operator:excluded",
		"tailapp.url.exclusion.kind":    "host-exact",
		"tailapp.url.exclusion.pattern": "excluded.example",
		"tailapp.url.exclusion.enabled": true,
	})); err != nil {
		t.Fatal(err)
	}

	frontier, err := materialized.Frontier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := Open(Namespace{Path: path, Profile: compiled, Frontier: frontier}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Close()

	due, err := sandbox.Query(context.Background(), Request{
		SQL: `SELECT o.observed_full
FROM url_observations AS o
LEFT JOIN url_verdicts AS v
  ON v.observed_full = o.observed_full AND v.provider = ?
WHERE v.observed_full IS NULL
   OR CAST(v.valid_until_unix_nano AS INTEGER) <= CAST(? AS INTEGER)
ORDER BY o.observed_full
LIMIT 200`,
		Parameters: []any{"safe-browsing-v5", "1788192000000000001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://due.example/no-verdict",
		"https://excluded.example/path",
		"https://expired.example/path",
		"https://other.example/path",
	}
	if len(due.Rows) != len(want) {
		t.Fatalf("due rows = %#v", due.Rows)
	}
	for index, expected := range want {
		if due.Rows[index][0] != expected {
			t.Fatalf("due rows = %#v", due.Rows)
		}
	}

	exclusions, err := sandbox.Query(context.Background(), Request{SQL: `SELECT exclusion_id, kind, pattern
FROM url_exclusions
WHERE enabled = 1
ORDER BY exclusion_id
LIMIT 200`})
	if err != nil {
		t.Fatal(err)
	}
	if len(exclusions.Rows) != 8 {
		t.Fatalf("enabled exclusions = %#v", exclusions.Rows)
	}
	foundCustom := false
	for _, row := range exclusions.Rows {
		if row[0] == "operator:excluded" && row[1] == "host-exact" && row[2] == "excluded.example" {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Fatalf("custom exclusion absent: %#v", exclusions.Rows)
	}

	counts, err := sandbox.Query(context.Background(), Request{SQL: `SELECT event_family, record_count
FROM url_pipeline_counts
ORDER BY event_family`})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(counts.Rows) != "[[exclusion 1] [observation 5] [reputation 3]]" {
		t.Fatalf("atomic pipeline counts = %#v", counts.Rows)
	}
}

func TestURLReputationDueQueryHasHardBound(t *testing.T) {
	compiled, err := tailapps.Load("url-reputation")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "url-reputation.sqlite")
	materialized, err := projection.Create(context.Background(), path, compiled, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer materialized.Close()
	for position := int64(1); position <= 320; position++ {
		url := fmt.Sprintf("https://host-%03d.example/path", position)
		item := delivery(position, "tailapp.url.observed", map[string]any{
			"tailapp.url.observed_full": url,
			"tailapp.url.host":          fmt.Sprintf("host-%03d.example", position),
		})
		if _, err := materialized.Process(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	frontier, err := materialized.Frontier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := Open(Namespace{Path: path, Profile: compiled, Frontier: frontier}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Close()
	result, err := sandbox.Query(context.Background(), Request{SQL: `SELECT o.observed_full
FROM url_observations AS o
LEFT JOIN url_verdicts AS v
  ON v.observed_full = o.observed_full AND v.provider = ?
WHERE v.observed_full IS NULL
   OR CAST(v.valid_until_unix_nano AS INTEGER) <= CAST(? AS INTEGER)
ORDER BY o.observed_full
LIMIT 200`, Parameters: []any{"safe-browsing-v5", "1788192000000000001"}, RowLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 200 || result.Truncated {
		t.Fatalf("bounded result rows=%d truncated=%v", len(result.Rows), result.Truncated)
	}
}
