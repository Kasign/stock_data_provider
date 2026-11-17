package domain

type TemplateFormat string

const (
	TemplateFormatText TemplateFormat = "text"
	TemplateFormatHTML TemplateFormat = "html"
)

// RenderedContent is the immutable output snapshot produced from a fixed template version.
type RenderedContent struct {
	TemplateID              string
	TemplateVersion         int64
	TemplateDigest          string
	EventPayloadFingerprint string
	Format                  TemplateFormat
	Subject                 string
	Body                    string
}
