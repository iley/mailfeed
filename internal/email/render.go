package email

import (
	"bytes"
	"html/template"
	"strings"
	texttemplate "text/template"

	"golang.org/x/net/html"

	"github.com/iley/mailfeed/internal/feed"
)

var (
	htmlTmpl = template.Must(template.New("email").Parse(rawHTMLTmpl))
	textTmpl = texttemplate.Must(texttemplate.New("email").Parse(rawTextTmpl))
)

type htmlView struct {
	FeedName    string
	Title       string
	Link        string
	Date        string
	Content     template.HTML
}

type textView struct {
	FeedName string
	Title    string
	Link     string
	Date     string
	Content  string
}

func toHTMLView(item feed.Item) htmlView {
	var date string
	if !item.PublishedAt.IsZero() {
		date = item.PublishedAt.Format("January 2, 2006")
	}
	return htmlView{
		FeedName: item.FeedName,
		Title:    item.Title,
		Link:     item.Link,
		Date:     date,
		Content:  template.HTML(item.Content),
	}
}

func toTextView(item feed.Item) textView {
	var date string
	if !item.PublishedAt.IsZero() {
		date = item.PublishedAt.Format("January 2, 2006")
	}
	return textView{
		FeedName: item.FeedName,
		Title:    item.Title,
		Link:     item.Link,
		Date:     date,
		Content:  stripHTML(item.Content),
	}
}

func RenderHTML(item feed.Item) (string, error) {
	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, toHTMLView(item)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func RenderPlainText(item feed.Item) (string, error) {
	var buf bytes.Buffer
	if err := textTmpl.Execute(&buf, toTextView(item)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func stripHTML(s string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var buf strings.Builder
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(buf.String())
		case html.TextToken:
			buf.WriteString(tokenizer.Token().Data)
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tokenizer.Token()
			switch t.Data {
			case "br", "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr":
				buf.WriteString("\n")
			}
		}
	}
}

const rawHTMLTmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0; padding:0; background:#f4f4f4; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f4;">
    <tr>
      <td align="center" style="padding:20px 10px;">
        <table width="600" cellpadding="0" cellspacing="0" style="max-width:600px; width:100%; background:#ffffff;">
          <!-- Header -->
          <tr>
            <td style="padding:20px 24px; border-bottom:2px solid #e0e0e0;">
              <span style="font-size:13px; color:#999;">{{.FeedName}}</span>
            </td>
          </tr>
          <!-- Title -->
          <tr>
            <td style="padding:24px 24px 0;">
              <h1 style="margin:0 0 8px; font-size:22px; line-height:1.3;">
                <a href="{{.Link}}" style="color:#1a1a1a; text-decoration:none;">{{.Title}}</a>
              </h1>
              {{if .Date}}<p style="margin:0; font-size:13px; color:#999;">{{.Date}}</p>{{end}}
            </td>
          </tr>
          <!-- Content -->
          <tr>
            <td style="padding:20px 24px 24px; font-size:16px; line-height:1.6; color:#333;">
              {{.Content}}
            </td>
          </tr>
          <!-- Footer -->
          <tr>
            <td style="padding:16px 24px; border-top:1px solid #eee; text-align:center;">
              <a href="{{.Link}}" style="font-size:13px; color:#1a73e8; text-decoration:none;">View original</a>
              <span style="color:#ccc; margin:0 8px;">·</span>
              <span style="font-size:12px; color:#bbb;">mailfeed</span>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

const rawTextTmpl = `{{.Title}}
{{.FeedName}}{{if .Date}} — {{.Date}}{{end}}
{{.Link}}

{{.Content}}

--
mailfeed`
