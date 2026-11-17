package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/pythonapi"
	appconfig "example.com/stock-analysis-flow/internal/platform/config"
)

type smokeOutput struct {
	BaseURL      string          `json:"base_url"`
	Stock        string          `json:"stock"`
	FetchedAt    time.Time       `json:"fetched_at"`
	Quote        *quoteSummary   `json:"quote,omitempty"`
	History      *historySummary `json:"history,omitempty"`
	Profile      map[string]any  `json:"profile,omitempty"`
	DataOverview json.RawMessage `json:"data_overview,omitempty"`
	Errors       []string        `json:"errors,omitempty"`
	Limitations  []string        `json:"limitations,omitempty"`
}

type quoteSummary struct {
	Name          string    `json:"name,omitempty"`
	CurrentPrice  float64   `json:"current_price"`
	ChangePercent *float64  `json:"change_percent,omitempty"`
	UpdateTime    string    `json:"update_time,omitempty"`
	FetchedAt     time.Time `json:"fetched_at"`
}

type historySummary struct {
	Count       int       `json:"count"`
	FirstDate   string    `json:"first_date,omitempty"`
	LatestDate  string    `json:"latest_date,omitempty"`
	LatestClose *float64  `json:"latest_close,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

func main() {
	var (
		stock           = flag.String("stock", "600519", "stock code, e.g. 600519")
		envFile         = flag.String("env-file", ".env", "environment file path")
		days            = flag.Int("history-days", 0, "override PYTHON_API_HISTORY_DAYS (1..365)")
		includeProfile  = flag.Bool("profile", false, "also call aggregated /profile endpoint")
		includeOverview = flag.Bool("overview", false, "also call /api/v1/data/overview diagnostic endpoint")
	)
	flag.Parse()

	if err := appconfig.LoadDotEnvIfExists(*envFile); err != nil {
		fail(err)
	}
	cfg, err := pythonapi.ConfigFromEnv()
	if err != nil {
		fail(err)
	}
	if *days != 0 {
		if *days < 1 || *days > 365 {
			fail(errors.New("--history-days must be between 1 and 365"))
		}
		cfg.HistoryDays = *days
	}
	client, err := pythonapi.NewClient(cfg)
	if err != nil {
		fail(err)
	}

	requestCount := 2 // quote + history
	if *includeProfile {
		requestCount++
	}
	if *includeOverview {
		requestCount++
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(requestCount+1))
	defer cancel()
	output := smokeOutput{
		BaseURL: cfg.BaseURL, Stock: strings.TrimSpace(*stock), FetchedAt: time.Now().UTC(),
		Limitations: []string{
			"该命令只验证 Python API 的连通性与响应契约，不运行 Go 因子/策略/风控。",
			"旧版 Python quote/history 响应未完整提供底层 Provider、available_at 和复权口径；输出不能直接用于 point-in-time 回测。",
		},
	}

	quote, quoteFetchedAt, quoteErr := client.GetQuote(ctx, output.Stock)
	if quoteErr != nil {
		output.Errors = append(output.Errors, "quote: "+quoteErr.Error())
	} else {
		output.Quote = &quoteSummary{
			Name: quote.StockName, CurrentPrice: quote.CurrentPrice, ChangePercent: quote.ChangePct,
			UpdateTime: quote.UpdateTime, FetchedAt: quoteFetchedAt,
		}
	}

	history, historyErr := client.GetHistory(ctx, output.Stock, cfg.HistoryDays)
	if historyErr != nil {
		output.Errors = append(output.Errors, "history: "+historyErr.Error())
	} else {
		var latestClose *float64
		// Upstream typically returns ascending bars, but compute dates independent of response order.
		firstDate, latestDate := "", ""
		for _, row := range history.Bars {
			if firstDate == "" || row.Date < firstDate {
				firstDate = row.Date
			}
			if latestDate == "" || row.Date > latestDate {
				latestDate = row.Date
				latestClose = row.Close
			}
		}
		output.History = &historySummary{Count: len(history.Bars), FirstDate: firstDate, LatestDate: latestDate, LatestClose: latestClose, FetchedAt: history.FetchedAt}
	}

	if *includeProfile {
		profile, profileErr := client.GetProfile(ctx, output.Stock, cfg.HistoryDays)
		if profileErr != nil {
			output.Errors = append(output.Errors, "profile: "+profileErr.Error())
		} else {
			var raw map[string]any
			if err := json.Unmarshal(mustJSON(profile), &raw); err == nil {
				output.Profile = map[string]any{
					"canonical_code":   raw["canonical_code"],
					"market":           raw["market"],
					"as_of":            raw["as_of"],
					"evidence_quality": raw["evidence_quality"],
					"quote":            blockStatus(raw["quote"]),
					"history":          blockStatus(raw["history"]),
					"research":         blockStatus(raw["research"]),
					"intelligence":     blockStatus(raw["intelligence"]),
					"portfolio":        blockStatus(raw["portfolio"]),
					"monitors":         blockStatus(raw["monitors"]),
				}
			}
		}
	}
	if *includeOverview {
		overview, overviewErr := client.GetDataOverview(ctx)
		if overviewErr != nil {
			output.Errors = append(output.Errors, "data_overview: "+overviewErr.Error())
		} else {
			output.DataOverview = overview
		}
	}

	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fail(err)
	}
	fmt.Println(string(encoded))
	if len(output.Errors) > 0 {
		os.Exit(1)
	}
}

func blockStatus(value any) any {
	block, ok := value.(map[string]any)
	if !ok {
		return map[string]any{"status": "unknown"}
	}
	return map[string]any{"status": block["status"], "limitations": block["limitations"]}
}

func mustJSON(value any) []byte {
	payload, _ := json.Marshal(value)
	return payload
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "python API smoke test:", err)
	os.Exit(2)
}
