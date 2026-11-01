package songs

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidInput = errors.New("title is required")

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context) ([]Song, error) { return s.repo.List(ctx) }

func (s *Service) Get(ctx context.Context, id string) (Song, error) {
	if id == "" {
		return Song{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateSongInput) (Song, error) {
	if err := validate(in); err != nil {
		return Song{}, err
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id string, in CreateSongInput) (Song, error) {
	if err := validate(in); err != nil {
		return Song{}, err
	}
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id string) error { return s.repo.Delete(ctx, id) }

func validate(in CreateSongInput) error {
	if strings.TrimSpace(in.Title) == "" {
		return ErrInvalidInput
	}
	return nil
}
