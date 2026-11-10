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

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
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
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney stock announcements only support common A-share equities, got %q", rawSymbol)
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
		addEastmoneyAnnouncementResponseIssue(ctx, "announcements", "HTTP response", "request_error", "successful announcement JSON response", "request_error", "东方财富公告请求或响应解码失败")
		return nil, fmt.Errorf("Eastmoney stock-announcement request: %w", err)
	}
	if payload.Success != 1 || payload.Data == nil {
		message := strings.TrimSpace(payload.Message)
		code := eastmoneyAnnouncementScalar(payload.Code)
		if payload.Data == nil && (message == "返回数据为空" || code == "9201") {
			addEastmoneyAnnouncementResponseIssue(ctx, "announcements", "data", "empty_result", "announcement data or explicit empty-result response", "empty", "东方财富公告接口返回明确空结果标记；空结果保持未知质量状态")
			return emptyEastmoneyAnnouncementSnapshot(symbol, startDate, endDate, page, pageSize), nil
		}
		addEastmoneyAnnouncementResponseIssue(ctx, "provider_response", "success/code/message", "provider_error", "success=1 with data object", fmt.Sprintf("success=%d code=%s", payload.Success, code), "东方财富公告接口拒绝请求或返回不完整响应")
		return nil, fmt.Errorf("Eastmoney stock-announcement response rejected: success=%d code=%q message=%q", payload.Success, code, message)
	}
	// 源端若回传分页元数据，必须与本次请求一致；不使用响应缺失字段伪造源端确认。
	if payload.Data.PageIndex > 0 && payload.Data.PageIndex != page {
		addEastmoneyAnnouncementResponseIssue(ctx, "page", "data.page_index", "pagination_metadata_mismatch", fmt.Sprintf("requested page %d", page), strconv.Itoa(payload.Data.PageIndex), "东方财富公告响应页码与请求页码不一致")
		return nil, fmt.Errorf("Eastmoney announcement page_index=%d does not match requested page=%d", payload.Data.PageIndex, page)
	}
	if payload.Data.PageSize > 0 && payload.Data.PageSize != pageSize {
		addEastmoneyAnnouncementResponseIssue(ctx, "page_size", "data.page_size", "pagination_metadata_mismatch", fmt.Sprintf("requested page size %d", pageSize), strconv.Itoa(payload.Data.PageSize), "东方财富公告响应分页大小与请求不一致")
		return nil, fmt.Errorf("Eastmoney announcement page_size=%d does not match requested page_size=%d", payload.Data.PageSize, pageSize)
	}
	total := payload.Data.TotalHits
	if total < 0 {
		addEastmoneyAnnouncementResponseIssue(ctx, "total", "data.total_hits", "invalid_pagination_total", "non-negative total_hits", strconv.Itoa(total), "东方财富公告响应 total_hits 为负数")
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
			addEastmoneyAnnouncementResponseIssue(ctx, "total", "data.total_hits/list", "pagination_metadata_mismatch", "total_hits consistent with non-empty list", "zero_with_rows", "东方财富公告 total_hits 为零但仍返回公告行")
			return nil, errors.New("Eastmoney announcement reports total_hits=0 with non-empty rows")
		}
		addEastmoneyAnnouncementResponseIssue(ctx, "announcements", "data.total_hits/list", "empty_result", "at least one announcement row", "empty", "东方财富公告查询返回零条记录；空结果保持未知质量状态")
		return emptyEastmoneyAnnouncementSnapshot(symbol, startDate, endDate, page, pageSize), nil
	}
	if page > pages {
		addEastmoneyAnnouncementResponseIssue(ctx, "page", "data.total_hits", "pagination_page_out_of_range", fmt.Sprintf("page <= %d", pages), strconv.Itoa(page), "请求页码超出东方财富公告源端总页数")
		return nil, fmt.Errorf("requested announcement page %d exceeds source page count %d", page, pages)
	}
	expectedRows := total - (page-1)*pageSize
	if expectedRows > pageSize {
		expectedRows = pageSize
	}
	if expectedRows <= 0 || len(payload.Data.List) != expectedRows {
		addEastmoneyAnnouncementResponseIssue(ctx, "rows", "data.list", "pagination_row_count_mismatch", strconv.Itoa(expectedRows), strconv.Itoa(len(payload.Data.List)), "东方财富公告当前页实际行数与 total_hits/page_size 推导值不一致")
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
			addEastmoneyAnnouncementParseIssue(ctx, index, "article", "art_code/title", "missing_required_value", "non-empty article ID and title", "missing", "东方财富公告行缺少公告 ID 或标题")
			return nil, fmt.Errorf("Eastmoney announcement row %d is missing article ID/title", index)
		}
		parsedNoticeDate, parseErr := time.Parse("2006-01-02", noticeDate)
		if parseErr != nil || parsedNoticeDate.Format("2006-01-02") != noticeDate {
			addEastmoneyAnnouncementParseIssue(ctx, index, "announcement_date", "notice_date", "invalid_date", "valid YYYY-MM-DD date", "invalid", "东方财富公告行公告日期缺失或格式无效")
			return nil, fmt.Errorf("Eastmoney announcement row %d has invalid notice_date %q", index, noticeDate)
		}
		if noticeDate < startDate || noticeDate > endDate {
			addEastmoneyAnnouncementParseIssue(ctx, index, "announcement_date", "notice_date", "date_out_of_requested_range", startDate+".."+endDate, noticeDate, "东方财富公告行日期超出请求区间")
			return nil, fmt.Errorf("Eastmoney announcement row %d date %s is outside requested range %s..%s", index, noticeDate, startDate, endDate)
		}
		if _, exists := seen[articleID]; exists {
			addEastmoneyAnnouncementParseIssue(ctx, index, "article_id", "art_code", "duplicate_article_id", "unique article ID within page", "duplicate", "东方财富公告当前页包含重复公告 ID")
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
			addEastmoneyAnnouncementParseIssue(ctx, index, "symbol", "codes[].stock_code", "symbol_mismatch", stockCode, "missing_or_mismatch", "东方财富公告行未关联本次请求的证券代码")
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
		if publishedAt == nil {
			addEastmoneyAnnouncementIssue(ctx, index, "published_at", "display_time", "missing_or_invalid_value", "precise provider publication timestamp", "string", displayTime, "东方财富公告展示时间缺失或无法解析；不使用公告日期伪造发布时间")
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


func addEastmoneyAnnouncementResponseIssue(ctx context.Context, field, providerField, code, expected, actualType, message string) {
	addEastmoneyAnnouncementIssueWithSeverity(ctx, -1, validation.SeverityError, field, providerField, code, expected, actualType, "", message)
}

func addEastmoneyAnnouncementParseIssue(ctx context.Context, rowIndex int, field, providerField, code, expected, actualType, message string) {
	addEastmoneyAnnouncementIssueWithSeverity(ctx, rowIndex, validation.SeverityError, field, providerField, code, expected, actualType, "", message)
}

func addEastmoneyAnnouncementIssue(ctx context.Context, rowIndex int, field, providerField, code, expected, actualType, preview, message string) {
	addEastmoneyAnnouncementIssueWithSeverity(ctx, rowIndex, validation.SeverityWarning, field, providerField, code, expected, actualType, preview, message)
}

func addEastmoneyAnnouncementIssueWithSeverity(ctx context.Context, rowIndex int, severity validation.Severity, field, providerField, code, expected, actualType, preview, message string) {
	report := validation.NewReport(8)
	issue := validation.FieldIssue{
		Stage: validation.StageValidate, Field: field, ProviderField: providerField,
		Code: code, Expected: expected, ActualType: actualType,
		ActualPreview: quoteIssuePreview(preview), Severity: severity,
		Message: message, RowIndex: &rowIndex,
	}
	report.Add(issue)
	validation.AddContextReport(ctx, report)
}
