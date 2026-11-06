package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// TestAsFormatCost: usage on models with no known price shows a localized
// "Not priced" instead of a fake $0; a partially priced usage keeps the "~"
// lower-bound marker; a fully priced one shows the plain amount.
func TestAsFormatCost(t *testing.T) {
	cases := []struct {
		name string
		u    agentsessions.Usage
		lang string
		want string
	}{
		{"unpriced en", agentsessions.Usage{Input: 10, UnknownModel: true}, "en", "Not priced"},
		{"unpriced es", agentsessions.Usage{Input: 10, UnknownModel: true}, "es", "Sin precio"},
		{"partial", agentsessions.Usage{CostUSD: 1.5, UnknownModel: true}, "en", "$1.50~"},
		{"priced", agentsessions.Usage{CostUSD: 1.5}, "en", "$1.50"},
		{"tiny priced", agentsessions.Usage{CostUSD: 0.004}, "en", "<$0.01"},
		{"no usage", agentsessions.Usage{}, "en", "—"},
	}
	for _, c := range cases {
		if got := asFormatCost(c.u, c.lang); got != c.want {
			t.Errorf("%s: asFormatCost = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestAsCostTitle: the tooltip explains an unpriced cost and names the
// models; priced or partially priced usage needs no tooltip.
func TestAsCostTitle(t *testing.T) {
	unpriced := agentsessions.Usage{Input: 10, UnknownModel: true}
	if got := asCostTitle(unpriced, []string{"gpt-5.6-sol", "gpt-5.6-terra"}, "en"); got != "No price is known for these models (gpt-5.6-sol, gpt-5.6-terra)." {
		t.Errorf("asCostTitle en = %q", got)
	}
	if got := asCostTitle(unpriced, nil, "es"); got != "No se conoce el precio de estos modelos." {
		t.Errorf("asCostTitle es without models = %q", got)
	}
	if got := asCostTitle(agentsessions.Usage{CostUSD: 1, UnknownModel: true}, []string{"x"}, "en"); got != "" {
		t.Errorf("partial cost must not carry the unpriced tooltip, got %q", got)
	}
}

// TestOvFormatSpend: the overview spend reads "Not priced" when every
// session this month ran on unpriced models, $0.00 for no spend and "~" for
// a lower bound.
func TestOvFormatSpend(t *testing.T) {
	cases := []struct {
		u    agentsessions.Usage
		lang string
		want string
	}{
		{agentsessions.Usage{UnknownModel: true}, "en", "Not priced"},
		{agentsessions.Usage{UnknownModel: true}, "es", "Sin precio"},
		{agentsessions.Usage{}, "en", "$0.00"},
		{agentsessions.Usage{CostUSD: 4.5, UnknownModel: true}, "en", "$4.50~"},
		{agentsessions.Usage{CostUSD: 4.5}, "en", "$4.50"},
	}
	for _, c := range cases {
		if got := ovFormatSpend(c.u, c.lang); got != c.want {
			t.Errorf("ovFormatSpend(%+v, %s) = %q, want %q", c.u, c.lang, got, c.want)
		}
	}
	sh := agentShare{Agent: "codex", Sessions: 3, Usage: agentsessions.Usage{UnknownModel: true}}
	if got := ovShareText(sh, "en", true); got != "Codex Not priced" {
		t.Errorf("ovShareText = %q, want %q", got, "Codex Not priced")
	}
	if strings.Contains(ovSplitLabel([]agentShare{sh}, "es", true), "$0") {
		t.Error("an unpriced share must not read as $0 in the split label")
	}
}

// TestAsCostCaption: an unpriced cost carries a visible caption naming up to
// three models then "+N"; priced or partially priced usage has none.
func TestAsCostCaption(t *testing.T) {
	unpriced := agentsessions.Usage{Input: 10, UnknownModel: true}
	cases := []struct {
		name   string
		u      agentsessions.Usage
		models []string
		lang   string
		want   string
	}{
		{"en two", unpriced, []string{"gpt-5.6-sol", "gpt-5.6-terra"}, "en", "No known price for gpt-5.6-sol, gpt-5.6-terra"},
		{"es one", unpriced, []string{"gpt-5.6-sol"}, "es", "Sin precio conocido para gpt-5.6-sol"},
		{"truncated", unpriced, []string{"a", "b", "c", "d", "e"}, "en", "No known price for a, b, c +2"},
		{"no models es", unpriced, nil, "es", "No se conoce el precio de estos modelos."},
		{"partial", agentsessions.Usage{CostUSD: 1, UnknownModel: true}, []string{"x"}, "en", ""},
		{"priced", agentsessions.Usage{CostUSD: 1}, nil, "en", ""},
	}
	for _, c := range cases {
		if got := asCostCaption(c.u, c.models, c.lang); got != c.want {
			t.Errorf("%s: asCostCaption = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestNotPricedCaption_Rendered: the overview spend card and the session
// detail cost field show the explanation as visible text, not only a title.
func TestNotPricedCaption_Rendered(t *testing.T) {
	var ov bytes.Buffer
	k := overviewAgentKPIs{Spend: agentsessions.Usage{Input: 10, UnknownModel: true}}
	if err := OverviewAgentKPIsFragment(k, "en").Render(context.Background(), &ov); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ov.String(), `<p class="kpi-cap">No price is known for these models.</p>`) {
		t.Errorf("overview spend card lacks the visible not-priced caption:\n%s", ov.String())
	}
	var pricedOv bytes.Buffer
	if err := OverviewAgentKPIsFragment(overviewAgentKPIs{Spend: agentsessions.Usage{CostUSD: 2}}, "en").Render(context.Background(), &pricedOv); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pricedOv.String(), "No price is known") {
		t.Error("a priced spend must not carry the not-priced caption")
	}

	var det bytes.Buffer
	d := &agentsessions.Detail{Session: agentsessions.Session{
		Agent: "codex", Usage: agentsessions.Usage{Input: 10, UnknownModel: true},
		Models: []string{"gpt-5.6-sol", "gpt-5.6-terra"},
	}}
	if err := AgentSessionDetailPartial(d, "es").Render(context.Background(), &det); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(det.String(), `<span class="cc-detail-cap">Sin precio conocido para gpt-5.6-sol, gpt-5.6-terra</span>`) {
		t.Errorf("detail cost field lacks the visible not-priced caption:\n%s", det.String())
	}
}
