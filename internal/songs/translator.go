package songs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrTranslatorDisabled = errors.New("translation service is not configured")

// Translator is a thin LibreTranslate-format client.
// Any provider that speaks POST {base}/translate with {q,source,target,format}
// and returns {translatedText} will work (LibreTranslate, DeepL proxy, etc.).
type Translator struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewTranslator(baseURL, apiKey string) *Translator {
	return &Translator{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *Translator) Enabled() bool { return t != nil && t.baseURL != "" }

type translateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
	Format string `json:"format"`
	APIKey string `json:"api_key,omitempty"`
}

type translateResponse struct {
	TranslatedText string `json:"translatedText"`
	Error          string `json:"error,omitempty"`
}

func (t *Translator) Translate(ctx context.Context, text, source, target string) (string, error) {
	if !t.Enabled() {
		return "", ErrTranslatorDisabled
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("nothing to translate")
	}
	if source == target {
		return text, nil
	}
	req := translateRequest{Q: text, Source: source, Target: target, Format: "text", APIKey: t.apiKey}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/translate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("translate request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("translate: status %d: %s", resp.StatusCode, string(raw))
	}
	var out translateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("translate: decode: %w", err)
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	return out.TranslatedText, nil
}

// StripChordsForTranslation returns lyric-only text for a source in the given format.
func StripChordsForTranslation(source, format string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	switch format {
	case "chordpro", "":
		p := ParseChordPro(source)
		return p.Lyrics
	case "opensong":
		if p, err := ParseOpenSong(source); err == nil {
			return p.Lyrics
		}
	case "openlyrics":
		if p, err := ParseOpenLyrics(source); err == nil {
			return p.Lyrics
		}
	}
	return source
}
