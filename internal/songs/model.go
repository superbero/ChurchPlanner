package songs

import "time"

// Song is now metadata-only. Musical content lives on Arrangements.
type Song struct {
	ID                  string    `json:"id"`
	Title               string    `json:"title"`
	Author              string    `json:"author"`
	YouTubeURL          string    `json:"youtube_url"`
	CCLINumber          string    `json:"ccli_number"`
	CopyrightHolder     string    `json:"copyright_holder"`
	CopyrightDate       string    `json:"copyright_date"`
	Publisher           string    `json:"publisher"`
	Scripture           string    `json:"scripture"`
	Notes               string    `json:"notes"`
	Archived            bool      `json:"archived"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	NeverUsed           bool      `json:"never_used"`
	RecentlyAddedUnused bool      `json:"recently_added_unused"`

	// Metadata from default arrangement / history
	OriginalKey string `json:"original_key"`
	BPM         int    `json:"bpm"`
	LastUsedKey string `json:"last_used_key"`
}

type CreateSongInput struct {
	Title           string
	Author          string
	YouTubeURL      string
	CCLINumber      string
	CopyrightHolder string
	CopyrightDate   string
	Publisher       string
	Scripture       string
	Notes           string
	Archived        bool
}

type Filter struct {
	Query           string
	Key             string // matches arrangement original_key OR any named key
	TagSlugs        []string
	IncludeArchived bool
	Sort            string
}

type SongPage struct {
	Songs    []Song
	Total    int
	Page     int
	PageSize int
}

type SongSearchSuggestion struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Archived bool   `json:"archived"`
}

const (
	defaultSongPageSize = 10
	maxSongPageSize     = 15
)

func normalizeSongPageSize(pageSize int) int {
	if pageSize < 1 {
		return defaultSongPageSize
	}
	if pageSize > maxSongPageSize {
		return maxSongPageSize
	}
	return pageSize
}

// Arrangement is a version of a song.
type Arrangement struct {
	ID              string    `json:"id"`
	SongID          string    `json:"song_id"`
	Name            string    `json:"name"`
	BPM             int       `json:"bpm"`
	Meter           string    `json:"meter"`
	LengthSeconds   int       `json:"length_seconds"`
	Sequence        string    `json:"sequence"`
	Notes           string    `json:"notes"`
	OriginalKey     string    `json:"original_key"`
	SourceContent   string    `json:"source_content"`
	Format          string    `json:"format"`
	PrimaryLanguage string    `json:"primary_language"`
	Lyrics          string    `json:"lyrics"`
	Chords          string    `json:"chords"`
	IsDefault       bool      `json:"is_default"`
	Archived        bool      `json:"archived"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateArrangementInput struct {
	Name            string
	BPM             int
	Meter           string
	LengthSeconds   int
	Sequence        string
	Notes           string
	OriginalKey     string
	SourceContent   string
	Format          string
	PrimaryLanguage string
	Archived        bool

	// Lyrics and Chords are derived from SourceContent by ArrangementService.
	// They are not user input; the service fills them before calling the repo.
	Lyrics string
	Chords string
}

// ArrangementKey is a named pitch setting for an arrangement.
type ArrangementKey struct {
	ID            string    `json:"id"`
	ArrangementID string    `json:"arrangement_id"`
	Name          string    `json:"name"`
	Key           string    `json:"key"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ChartLine is one rendered line of a chart (chords above lyrics).
type ChartLine struct {
	Chords []ChordPos
	Lyric  string
}

type ChordPos struct {
	Position int
	Chord    string
}

// RenderedSection is a chart section ready for display.
type RenderedSection struct {
	Label string
	Lines []ChartLine
}

type ArrangementTranslation struct {
	ID            string
	ArrangementID string
	Language      string
	Lyrics        string
	Chords        string
	SourceContent string
	Format        string
	Notes         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Suggestion struct {
	SongID          string
	SongTitle       string
	SongAuthor      string
	ArrangementID   string
	ArrangementName string
	Key             string
	Score           float64
	Reason          string
	LastUsedDays    int
	Uses3m          int
}
