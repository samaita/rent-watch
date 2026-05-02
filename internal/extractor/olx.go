package extractor

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/axonigma/rent-watcher/internal/model"
)

type OLX struct {
	opts BrowserOptions
}

func NewOLX(opts BrowserOptions) *OLX {
	return &OLX{opts: opts}
}

type rawCard struct {
	URL      string `json:"url"`
	PhotoURL string `json:"photo_url"`
	Text     string `json:"text"`
	Title    string `json:"title"`
}

func (o *OLX) ExtractListings(parent context.Context, watch model.WatchPage) ([]model.ExtractedListing, error) {
	ctx, cancel := newBrowserContext(parent, o.opts)
	defer cancel()

	var cards []rawCard
	err := chromedp.Run(ctx,
		chromedp.Navigate(watch.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(olxCardsJS, &cards),
	)
	if err != nil {
		return nil, err
	}

	out := make([]model.ExtractedListing, 0, len(cards))
	for _, card := range cards {
		canonical := canonicalizeURL(card.URL)
		if canonical == "" {
			continue
		}
		extracted := parseOLXCard(card)
		extracted.CanonicalURL = canonical
		payload, _ := json.Marshal(card)
		extracted.RawJSON = string(payload)
		out = append(out, extracted)
	}
	return dedupe(out), nil
}

func dedupe(in []model.ExtractedListing) []model.ExtractedListing {
	seen := make(map[string]struct{}, len(in))
	out := make([]model.ExtractedListing, 0, len(in))
	for _, item := range in {
		if item.CanonicalURL == "" {
			continue
		}
		if _, ok := seen[item.CanonicalURL]; ok {
			continue
		}
		seen[item.CanonicalURL] = struct{}{}
		out = append(out, item)
	}
	return out
}

func parseOLXCard(card rawCard) model.ExtractedListing {
	lines := nonEmptyLines(card.Title + "\n" + card.Text)
	out := model.ExtractedListing{
		PhotoURL: card.PhotoURL,
	}
	if len(lines) > 0 {
		out.LocationKelurahan = detectKelurahan(lines)
	}
	out.Price = parsePrice(lines)
	out.SizeM2 = parseSize(lines)
	out.Bedrooms = parseCount(lines, `(?i)(\d+)\s*kt`)
	out.Bathrooms = parseCount(lines, `(?i)(\d+)\s*km`)
	return out
}

func nonEmptyLines(raw string) []string {
	split := strings.Split(raw, "\n")
	out := make([]string, 0, len(split))
	for _, line := range split {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func detectKelurahan(lines []string) string {
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "kel.") || strings.Contains(lower, "kelurahan") {
			return line
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 1 {
		return lines[len(lines)-1]
	}
	return lines[0]
}

func parsePrice(lines []string) *int64 {
	for _, line := range lines {
		if !strings.Contains(strings.ToLower(line), "rp") {
			continue
		}
		digits := digitsOnly(line)
		if digits == "" {
			continue
		}
		v, err := strconv.ParseInt(digits, 10, 64)
		if err == nil {
			return &v
		}
	}
	return nil
}

func parseSize(lines []string) *float64 {
	re := regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*m2|(\d+(?:[.,]\d+)?)\s*m²`)
	for _, line := range lines {
		match := re.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		raw := match[1]
		if raw == "" && len(match) > 2 {
			raw = match[2]
		}
		raw = strings.ReplaceAll(raw, ",", ".")
		v, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return &v
		}
	}
	return nil
}

func parseCount(lines []string, expr string) *int64 {
	re := regexp.MustCompile(expr)
	for _, line := range lines {
		match := re.FindStringSubmatch(line)
		if len(match) < 2 {
			continue
		}
		v, err := strconv.ParseInt(match[1], 10, 64)
		if err == nil {
			return &v
		}
	}
	return nil
}

func canonicalizeURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func digitsOnly(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

const olxCardsJS = `(function() {
  const anchors = Array.from(document.querySelectorAll('a[href]'));
  const looksLikeListing = (href) => {
    if (!href) return false;
    return href.includes('/item/') || href.includes('olx.co.id/item/');
  };
  const seen = new Set();
  const out = [];
  for (const a of anchors) {
    const href = a.href || '';
    if (!looksLikeListing(href) || seen.has(href)) continue;
    seen.add(href);
    const card = a.closest('li, article, div') || a;
    const img = card.querySelector('img');
    const title = a.textContent || '';
    const text = card.innerText || '';
    out.push({
      url: href,
      photo_url: img ? (img.src || img.getAttribute('src') || '') : '',
      title,
      text
    });
  }
  return out;
})()`
