package domain

import "time"

// NewsItemVersion 是不可变的来源内容版本快照；VersionNo 在单个新闻主体内单调递增。
type NewsItemVersion struct {
	ID              string
	NewsID          string
	VersionNo       int
	Fingerprint     string
	Title           string
	PublisherName   string
	PublisherDomain string
	Author          string
	PublishedAt     *time.Time
	Content         ContentLayers
	ObservedAt      time.Time
	ObservationID   string
}
