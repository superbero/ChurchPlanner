package songs

import "time"

type Song struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Key       string    `json:"key"`
	Tempo     int       `json:"tempo"`
	Lyrics    string    `json:"lyrics"`
	Chords    string    `json:"chords"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateSongInput struct {
	Title  string
	Author string
	Key    string
	Tempo  int
	Lyrics string
	Chords string
}
