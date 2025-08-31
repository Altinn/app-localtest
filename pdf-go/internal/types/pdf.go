package types

import "context"

const MaxConcurrency = 4

type PdfRequest struct {
	URL                 string     `json:"url"`
	Options             PdfOptions `json:"options"`
	SetJavaScriptEnabled bool       `json:"setJavaScriptEnabled"`
	WaitFor             string     `json:"waitFor"`
	Cookies             []Cookie   `json:"cookies"`
}

type PdfOptions struct {
	HeaderTemplate      string     `json:"headerTemplate"`
	FooterTemplate      string     `json:"footerTemplate"`
	DisplayHeaderFooter bool       `json:"displayHeaderFooter"`
	PrintBackground     bool       `json:"printBackground"`
	Format              string     `json:"format"`
	Margin              PdfMargin  `json:"margin"`
}

type PdfMargin struct {
	Top    string `json:"top"`
	Right  string `json:"right"`
	Bottom string `json:"bottom"`
	Left   string `json:"left"`
}

type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	SameSite string `json:"sameSite"`
}

type PdfResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type PdfResult struct {
	Data    []byte
	Browser BrowserVersion
}

type BrowserVersion struct {
	Product         string
	ProtocolVersion string
	Revision        string
	UserAgent       string
	JSVersion       string
}

type PdfGenerator interface {
	Generate(ctx context.Context, request PdfRequest) (*PdfResult, error)
	Close() error
}
