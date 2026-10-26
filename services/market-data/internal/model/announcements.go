package model

import "time"

// StockAnnouncement 表示东方财富返回的单只 A 股公告记录。
// announcement_date 是公告日期；published_at 仅在源端提供可解析展示时间时设置；fetched_at 永远单独记录采集时间。
type StockAnnouncement struct {
	ArticleID        string        `json:"article_id"`
	Symbol           string        `json:"symbol"`
	StockName        string        `json:"stock_name,omitempty"`
	Title            string        `json:"title"`
	TitleChinese     string        `json:"title_chinese,omitempty"`
	TitleEnglish     string        `json:"title_english,omitempty"`
	CategoryCode     string        `json:"category_code,omitempty"`
	Category         string        `json:"category,omitempty"`
	AnnouncementDate string        `json:"announcement_date"`
	PublishedAt      *time.Time    `json:"published_at,omitempty"`
	DisplayTime      string        `json:"display_time,omitempty"`
	SortDate         string        `json:"sort_date,omitempty"`
	EITime            string        `json:"ei_time,omitempty"`
	Language         string        `json:"language,omitempty"`
	SourceType       string        `json:"source_type,omitempty"`
	SourceURL        string        `json:"source_url,omitempty"`
	Source           string        `json:"source"`
	SourceVersion    string        `json:"source_version"`
	FetchedAt        time.Time     `json:"fetched_at"`
	QualityStatus    QualityStatus `json:"quality_status"`
	QualityReasons   []string      `json:"quality_reasons,omitempty"`
}

// StockAnnouncementSnapshot 表示一只股票在明确日期区间内的公告分页。
// total 为源端匹配总数；items 仅为请求页，不代表公告全集。
type StockAnnouncementSnapshot struct {
	Symbol         string             `json:"symbol"`
	StartDate      string             `json:"start_date"`
	EndDate        string             `json:"end_date"`
	Page           int                `json:"page"`
	PageSize       int                `json:"page_size"`
	Total          int                `json:"total"`
	Pages          int                `json:"pages"`
	Returned       int                `json:"returned"`
	HasData        bool               `json:"has_data"`
	HasMore        bool               `json:"has_more"`
	Items          []StockAnnouncement `json:"items"`
	Source         string             `json:"source"`
	SourceVersion  string             `json:"source_version"`
	FetchedAt      time.Time          `json:"fetched_at"`
	QualityStatus  QualityStatus      `json:"quality_status"`
	QualityReasons []string           `json:"quality_reasons,omitempty"`
}
