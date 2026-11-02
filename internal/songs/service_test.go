// internal/songs/service_test.go
package songs

import (
	"context"
	"testing"
)

func TestCreate_RejectsEmptyTitle(t *testing.T) {
	s := NewService(nil) // repo not needed: validation fails first
	_, err := s.Create(context.Background(), CreateSongInput{Title: "   "})
	if err != ErrInvalidInput {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}
