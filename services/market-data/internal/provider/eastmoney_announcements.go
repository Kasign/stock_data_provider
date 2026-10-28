package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const defaultEastmoneyAnnouncementURL = "https://np-anotice-stock.eastmoney.com/api/security/ann"

type eastmoneyAnnouncementEnvelope struct {
	Success int             `json:"success"`
	Message string          `json:"message"`
	Code    json.RawMessage `json:"code"`
	Data    *struct {
		List      []eastmoneyAnnouncementRow `json:"list"`
		PageIndex int                        `json:"page_index"`
		PageSize  int                        `json:"page_size"`
		TotalHits int                        `json:"total_hits"`
	} `json:"data"`
}

type eastmoneyAnnouncementRow struct {
	ArticleCode string `json:"art_code"`
	Codes       []struct {
		ShortName  string `json:"short_name"`
		StockCode  string `json:"stock_code"`
		MarketCode string `json:"market_code"`
	} `json:"codes"`
	Columns []struct {
		Code string `json:"column_code"`
		Name string `json:"column_name"`
	} `json:"columns"`
	DisplayTime  json.RawMessage `json:"display_time"`
	EITime       json.RawMessage `json:"eiTime"`
	NoticeDate   json.RawMessage `json:"notice_date"`
	SortDate     json.RawMessage `json:"sort_date"`
	Language     json.RawMessage `json:"language"`
	SourceType   json.RawMessage `json:"source_type"`
	Title        string          `json:"title"`
	TitleChinese string          `json:"title_ch"`
	TitleEnglish string          `json:"title_en"`
}

func eastmoneyAnnouncementScalar(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(string(raw))
}

func eastmoneyAnnouncementTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return nil
	}
	// 东方财富 display_time/eiTime 偶尔以“秒:毫秒”返回（例如 17:20:42:715）。
	// 只规范化末尾恰好 3 位数字的冒号分隔毫秒，不影响时区偏移中的冒号。
	if index := strings.LastIndex(raw, ":"); index >= 0 && len(raw)-index == 4 {
		milliseconds := raw[index+1:]
		if len(milliseconds) == 3 {
			validDigits := true
			for _, digit := range milliseconds {
				if digit < '0' || digit > '9' {
					validDigits = false
					break
				}
			}
			if validDigits {
				raw = raw[:index] + "." + milliseconds
			}
		}
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04:05"} {
		var parsed time.Time
		if layout == time.RFC3339Nano {
			parsed, err = time.Parse(layout, raw)
		} else {
			parsed, err = time.ParseInLocation(layout, raw, loc)
		}
		if err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	// 纯日期只表达 announcement_date，不伪装成精确发布时间。
	return nil
}

func eastmoneyAnnouncementURL(stockCode, articleID string) string {
	// 东方财富公开公告详情页的 URL 模式使用证券代码和 art_code 两段路径。
	return "https://data.eastmoney.com/notices/detail/" + url.PathEscape(stockCode) + "/" + url.PathEscape(articleID) + ".html"
}

// FetchStockAnnouncements 获取单只 A 股在含首尾日期的公告列表。
// 源端公告日期、展示时间和采集时间分别保存；不构造虚假的发布时间。
func (p *Eastmoney) FetchStockAnnouncements(ctx context.Context, rawSymbol, startDate, endDate string, page, pageSize int) (*model.StockAnnouncementSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	start, startErr := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	end, endErr := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if startErr != nil || endErr != nil || start.Format("2006-01-02") != strings.TrimSpace(startDate) ||
		end.Format("2006-01-02") != strings.TrimSpace(endDate) || start.After(end) {
		return nil, errors.New("announcement start/end dates must be valid YYYY-MM-DD dates with start_date <= end_date")
	}
	if end.Sub(start) > 365*24*time.Hour {
		return nil, errors.New("announcement date range must not exceed 366 days")
	}
	if page < 1 || page > 1000 {
		return nil, errors.New("announcement page must be between 1 and 1000")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, errors.New("announcement page_size must be between 1 and 100")
	}
	if strings.TrimSpace(p.announcementURL) == "" {
		return nil, errors.New("Eastmoney announcement endpoint is not configured")
	}
	stockCode := strings.Split(symbol, ".")[0]
	params := url.Values{
		"sr":            {"-1"},
		"page_size":     {strconv.Itoa(pageSize)},
		"page_index":    {strconv.Itoa(page)},
		"ann_type":      {"A"},
		"client_source": {"web"},
		"f_node":        {"0"},
		"s_node":        {"0"},
		"begin_time":    {startDate},
		"end_time":      {endDate},
		"stock_list":    {stockCode},
	}
	var payload eastmoneyAnnouncementEnvelope
	if err := p.getJSONWithRetry(ctx, p.announcementURL, params, &payload, 8*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney stock-announcement request: %w", err)
	}
	if payload.Success != 1 || payload.Data == nil {
		message := strings.TrimSpace(payload.Message)
		code := eastmoneyAnnouncementScalar(payload.Code)
		if payload.Data == nil && (message == "返回数据为空" || code == "9201") {
			return emptyEastmoneyAnnouncementSnapshot(symbol, startDate, endDate, page, pageSize), nil
		}
		return nil, fmt.Errorf("Eastmoney stock-announcement response rejected: success=%d code=%q message=%q", payload.Success, code, message)
	}
	total := payload.Data.TotalHits
	if total < 0 {
		return nil, fmt.Errorf("Eastmoney announcement total_hits is negative: %d", total)
	}
	fetchedAt := time.Now().UTC()
	pages := 0
	if total > 0 {
		// 使用减一后整除，避免 total 接近 int 上界时加法溢出。
		pages = 1 + (total-1)/pageSize
	}
	if total == 0 {
		if len(payload.Data.List) != 0 {
			return nil, errors.New("Eastmoney announcement reports total_hits=0 with non-empty rows")
		}
		return emptyEastmoneyAnnouncementSnapshot(symbol, startDate, endDate, page, pageSize), nil
	}
	if page > pages {
		return nil, fmt.Errorf("requested announcement page %d exceeds source page count %d", page, pages)
	}
	expectedRows := total - (page-1)*pageSize
	if expectedRows > pageSize {
		expectedRows = pageSize
	}
	if expectedRows <= 0 || len(payload.Data.List) != expectedRows {
		return nil, fmt.Errorf(
			"Eastmoney announcement page %d returned %d rows, expected %d from total_hits=%d and page_size=%d",
			page, len(payload.Data.List), expectedRows, total, pageSize,
		)
	}

	snapshot := &model.StockAnnouncementSnapshot{
		Symbol: symbol, StartDate: startDate, EndDate: endDate, Page: page, PageSize: pageSize,
		Total: total, Pages: pages, Returned: len(payload.Data.List), HasData: true, HasMore: page < pages,
		Items: make([]model.StockAnnouncement, 0, len(payload.Data.List)), Source: p.Name(),
		SourceVersion: "eastmoney-announcement-v1", FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
	seen := make(map[string]struct{}, len(payload.Data.List))
	for index, row := range payload.Data.List {
		articleID := strings.TrimSpace(row.ArticleCode)
		title := strings.TrimSpace(row.Title)
		if title == "" {
			title = strings.TrimSpace(row.TitleChinese)
		}
		if title == "" {
			title = strings.TrimSpace(row.TitleEnglish)
		}
		noticeDate := eastmoneyAnnouncementScalar(row.NoticeDate)
		if len(noticeDate) > 10 && noticeDate[4] == '-' && noticeDate[7] == '-' {
			noticeDate = noticeDate[:10]
		}
		if articleID == "" || title == "" {
			return nil, fmt.Errorf("Eastmoney announcement row %d is missing article ID/title", index)
		}
		parsedNoticeDate, parseErr := time.Parse("2006-01-02", noticeDate)
		if parseErr != nil || parsedNoticeDate.Format("2006-01-02") != noticeDate {
			return nil, fmt.Errorf("Eastmoney announcement row %d has invalid notice_date %q", index, noticeDate)
		}
		if noticeDate < startDate || noticeDate > endDate {
			return nil, fmt.Errorf("Eastmoney announcement row %d date %s is outside requested range %s..%s", index, noticeDate, startDate, endDate)
		}
		if _, exists := seen[articleID]; exists {
			return nil, fmt.Errorf("Eastmoney announcement page has duplicate art_code %q", articleID)
		}
		seen[articleID] = struct{}{}

		stockName := ""
		foundRequestedStock := len(row.Codes) == 0
		for _, code := range row.Codes {
			if strings.TrimSpace(code.StockCode) == stockCode {
				foundRequestedStock = true
				if stockName == "" {
					stockName = strings.TrimSpace(code.ShortName)
				}
			}
		}
		if !foundRequestedStock {
			return nil, fmt.Errorf("Eastmoney announcement row %d does not include requested stock %s in codes[]", index, stockCode)
		}
		categoryCode, category := "", ""
		if len(row.Columns) > 0 {
			categoryCode = strings.TrimSpace(row.Columns[0].Code)
			category = strings.TrimSpace(row.Columns[0].Name)
		}
		displayTime := eastmoneyAnnouncementScalar(row.DisplayTime)
		publishedAt := eastmoneyAnnouncementTime(displayTime)
		reasons := []string{}
		itemQuality := model.QualityValid
		if publishedAt == nil {
			itemQuality = model.QualityDegraded
			reasons = append(reasons, "provider_display_time_missing_or_unparseable")
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_display_time_missing_or_unparseable")
		}
		item := model.StockAnnouncement{
			ArticleID: articleID, Symbol: symbol, StockName: stockName, Title: title,
			TitleChinese: strings.TrimSpace(row.TitleChinese), TitleEnglish: strings.TrimSpace(row.TitleEnglish),
			CategoryCode: categoryCode, Category: category, AnnouncementDate: noticeDate, PublishedAt: publishedAt,
			DisplayTime: displayTime, SortDate: eastmoneyAnnouncementScalar(row.SortDate), EITime: eastmoneyAnnouncementScalar(row.EITime),
			Language: eastmoneyAnnouncementScalar(row.Language), SourceType: eastmoneyAnnouncementScalar(row.SourceType),
			SourceURL: eastmoneyAnnouncementURL(stockCode, articleID), Source: p.Name(), SourceVersion: snapshot.SourceVersion,
			FetchedAt: fetchedAt, QualityStatus: itemQuality, QualityReasons: reasons,
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, nil
}

func emptyEastmoneyAnnouncementSnapshot(symbol, startDate, endDate string, page, pageSize int) *model.StockAnnouncementSnapshot {
	return &model.StockAnnouncementSnapshot{
		Symbol: symbol, StartDate: startDate, EndDate: endDate, Page: page, PageSize: pageSize,
		Total: 0, Pages: 0, Returned: 0, HasData: false, HasMore: false, Items: []model.StockAnnouncement{},
		Source: "eastmoney", SourceVersion: "eastmoney-announcement-v1", FetchedAt: time.Now().UTC(),
		QualityStatus:  model.QualityUnknown,
		QualityReasons: []string{"provider_returned_empty_announcement_list_not_confirmed_as_no_announcements"},
	}
}
