package songs

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidInput = errors.New("title is required")

type Service struct {
	repo    *Repository
	history *HistoryRepository
}

func NewService(repo *Repository, history *HistoryRepository) *Service {
	return &Service{repo: repo, history: history}
}

func (s *Service) List(ctx context.Context) ([]Song, error)             { return s.repo.List(ctx) }
func (s *Service) Search(ctx context.Context, f Filter) ([]Song, error) { return s.repo.Search(ctx, f) }
func (s *Service) SearchSuggestions(ctx context.Context, query string, includeArchived bool, limit int) ([]SongSearchSuggestion, error) {
	return s.repo.SearchSuggestions(ctx, query, includeArchived, limit)
}
func (s *Service) SearchPage(ctx context.Context, f Filter, page, pageSize int) (SongPage, error) {
	return s.repo.SearchPage(ctx, f, page, pageSize)
}
func (s *Service) Get(ctx context.Context, id string) (Song, error) {
	if id == "" {
		return Song{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}
func (s *Service) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	if strings.TrimSpace(in.Title) == "" {
		return Song{}, ErrInvalidInput
	}
	return s.repo.Create(ctx, in)
}
func (s *Service) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	if strings.TrimSpace(in.Title) == "" {
		return Song{}, ErrInvalidInput
	}
	return s.repo.Update(ctx, id, in)
}
func (s *Service) Delete(ctx context.Context, id string) error { return s.repo.Delete(ctx, id) }
func (s *Service) DeleteMany(ctx context.Context, ids []string) error {
	return s.repo.DeleteMany(ctx, ids)
}

func (s *Service) History(ctx context.Context, songID string) ([]KeyHistory, error) {
	return s.history.List(ctx, songID)
}
func (s *Service) AddHistory(ctx context.Context, h KeyHistory) error {
	return s.history.Add(ctx, h)
}
