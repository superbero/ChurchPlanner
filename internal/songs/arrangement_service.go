package songs

import (
	"context"
	"errors"
	"strings"
)

type ArrangementService struct {
	repo *ArrangementRepository
}

func NewArrangementService(repo *ArrangementRepository) *ArrangementService {
	return &ArrangementService{repo: repo}
}

func (s *ArrangementService) List(ctx context.Context, songID string) ([]Arrangement, error) {
	return s.repo.ListBySong(ctx, songID)
}

func (s *ArrangementService) Get(ctx context.Context, id string) (Arrangement, error) {
	return s.repo.Get(ctx, id)
}

func (s *ArrangementService) Default(ctx context.Context, songID string) (Arrangement, error) {
	return s.repo.DefaultForSong(ctx, songID)
}

func (s *ArrangementService) Create(ctx context.Context, songID string, in CreateArrangementInput, isDefault bool) (Arrangement, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Arrangement{}, errors.New("arrangement name is required")
	}
	in.Lyrics, in.Chords = parseChart(in.SourceContent, in.Format)
	return s.repo.Create(ctx, songID, in, isDefault)
}

func (s *ArrangementService) Update(ctx context.Context, id string, in CreateArrangementInput) (Arrangement, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Arrangement{}, errors.New("arrangement name is required")
	}
	lyrics, chords := parseChart(in.SourceContent, in.Format)
	return s.repo.Update(ctx, id, in, lyrics, chords)
}

func (s *ArrangementService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *ArrangementService) SetDefault(ctx context.Context, songID, arrangementID string) error {
	return s.repo.SetDefault(ctx, songID, arrangementID)
}

// Keys returns the keys for an arrangement, or an empty list if none.
func (s *ArrangementService) Keys(ctx context.Context, arrangementID string) ([]ArrangementKey, error) {
	return s.repo.KeysForArrangement(ctx, arrangementID)
}

func (s *ArrangementService) DefaultKey(ctx context.Context, arrangementID string) (ArrangementKey, error) {
	return s.repo.DefaultKeyForArrangement(ctx, arrangementID)
}

func (s *ArrangementService) CreateKey(ctx context.Context, arrangementID, name, key string, isDefault bool) (ArrangementKey, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(key) == "" {
		return ArrangementKey{}, errors.New("key name and value are required")
	}
	return s.repo.CreateKey(ctx, arrangementID, strings.TrimSpace(name), strings.TrimSpace(key), isDefault)
}

func (s *ArrangementService) DeleteKey(ctx context.Context, arrangementID, keyID string) error {
	// Refuse to delete the last key.
	keys, err := s.repo.KeysForArrangement(ctx, arrangementID)
	if err != nil {
		return err
	}
	if len(keys) <= 1 {
		return errors.New("cannot delete the only key")
	}
	return s.repo.DeleteKey(ctx, keyID)
}

func (s *ArrangementService) SetDefaultKey(ctx context.Context, arrangementID, keyID string) error {
	return s.repo.SetDefaultKey(ctx, arrangementID, keyID)
}

// Chart is a backwards-compatible wrapper that returns the chart in the
// arrangement's primary language.
func (s *ArrangementService) Chart(ctx context.Context, arrangementID, displayKey string) ([]RenderedSection, string, error) {
	chart, dk, _, _, err := s.ChartForLanguage(ctx, arrangementID, displayKey, "")
	return chart, dk, err
}

// RenderPDF builds a PDF for the arrangement, honoring the requested language.
func (s *ArrangementService) RenderPDF(ctx context.Context, song *Song, arrangementID, displayKey, qrURL, lang, pdfFormat string) ([]byte, string, error) {
	a, err := s.repo.Get(ctx, arrangementID)
	if err != nil {
		return nil, "", err
	}
	chart, dk, resolvedLang, _, err := s.ChartForLanguage(ctx, arrangementID, displayKey, lang)
	if err != nil {
		return nil, "", err
	}
	data, err := RenderSongPDF(song.Title, song.Author, dk, a.Name, a.Sequence, chart, qrURL, resolvedLang, pdfFormat)
	if err != nil {
		return nil, "", err
	}
	suffix := ""
	if resolvedLang != "" && resolvedLang != a.PrimaryLanguage {
		suffix = "_" + resolvedLang
	}
	return data, sanitizeFilename(song.Title+"_"+a.Name) + suffix + ".pdf", nil
}

// RestoreRevision (delegates to the repository; also snapshots current state).
func (s *ArrangementService) RestoreRevision(ctx context.Context, arrangementID, revisionID string) (Arrangement, error) {
	return s.repo.RestoreRevision(ctx, arrangementID, revisionID)
}

// parseChart returns (lyrics, chords) with line alignment based on the format.
func parseChart(source, format string) (lyrics, chords string) {
	if strings.TrimSpace(source) == "" {
		return "", ""
	}
	switch format {
	case "chordpro", "":
		p := ParseChordPro(source)
		return p.Lyrics, p.Chords
	case "opensong":
		if p, err := ParseOpenSong(source); err == nil {
			return p.Lyrics, p.Chords
		}
	case "openlyrics":
		if p, err := ParseOpenLyrics(source); err == nil {
			return p.Lyrics, p.Chords
		}
	}
	return source, ""
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '/':
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "song"
	}
	return b.String()
}


// SearchArrangements returns (song, arrangement) pairs matching q.
// Empty q returns the first `limit` pairs alphabetically.
func (s *ArrangementService) SearchArrangements(ctx context.Context, q string, limit int) ([]ArrangementSearchResult, error) {
	return s.repo.SearchArrangements(ctx, q, limit)
}
