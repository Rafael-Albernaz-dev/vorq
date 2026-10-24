package app_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type blockingDiscovery struct {
	app.Repository
	active, max atomic.Int32
	started     chan struct{}
}

func (r *blockingDiscovery) ProjectPaths(context.Context, string) (string, string, error) {
	return "/workspace", "/workspace/.git", nil
}
func (r *blockingDiscovery) ReadWorktrees(context.Context, string) ([]*domain.Workspace, error) {
	result := make([]*domain.Workspace, 20)
	for i := range result {
		result[i] = &domain.Workspace{Path: fmt.Sprintf("/workspace/%d", i)}
	}
	return result, nil
}
func (r *blockingDiscovery) ReadChanges(ctx context.Context, dir, branch string) (*domain.ChangeSet, error) {
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for old := r.max.Load(); active > old; old = r.max.Load() {
		if r.max.CompareAndSwap(old, active) {
			break
		}
	}
	r.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestDiscoveryBoundsWorkersAndJoinsOnCancellation(t *testing.T) {
	repo := &blockingDiscovery{started: make(chan struct{}, 20)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := app.NewApp(repo).ListWorktrees(ctx, "/workspace"); done <- err }()
	for i := 0; i < 4; i++ {
		select {
		case <-repo.started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery workers did not exit")
	}
	if repo.max.Load() > 4 || repo.active.Load() != 0 {
		t.Fatalf("worker bound/leak: max=%d active=%d", repo.max.Load(), repo.active.Load())
	}
}
