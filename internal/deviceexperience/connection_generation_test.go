package deviceexperience_test

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestV2GenerationDurableMonotonicAcrossRepositoryRestart(t *testing.T) {
	ctx := context.Background()
	first, handle, _ := newRepository(t)
	a, err := first.AllocateV2ConnectionGeneration(ctx)
	if err != nil || a < 1 {
		t.Fatalf("first durable generation=%d err=%v", a, err)
	}
	b, err := first.AllocateV2ConnectionGeneration(ctx)
	if err != nil || b != a+1 {
		t.Fatalf("second durable generation=%d previous=%d err=%v", b, a, err)
	}
	// A newly started Hub/Repository must not restart at 1 while the
	// previous process's ACK/transfer remains in SQLite.
	second, err := deviceexperience.NewRepository(handle.DB)
	if err != nil {
		t.Fatal(err)
	}
	c, err := second.AllocateV2ConnectionGeneration(ctx)
	if err != nil || c != b+1 {
		t.Fatalf("restart reused old generation=%d previous=%d err=%v", c, b, err)
	}
	var stored int64
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT generation FROM stage_device_v2_connection_sequence WHERE singleton=1").Scan(&stored); err != nil || stored != c {
		t.Fatalf("durable high-water=%d want=%d err=%v", stored, c, err)
	}
	if _, err := handle.DB.ExecContext(ctx,
		"UPDATE stage_device_v2_connection_sequence SET generation=? WHERE singleton=1", a); err == nil {
		t.Fatal("direct SQL rewound v2 connection generation")
	}
	if _, err := handle.DB.ExecContext(ctx,
		"DELETE FROM stage_device_v2_connection_sequence WHERE singleton=1"); err == nil {
		t.Fatal("direct SQL deleted v2 connection high-water")
	}
}

func TestV2GenerationFailsClosedAtInt64Exhaustion(t *testing.T) {
	ctx := context.Background()
	repo, handle, _ := newRepository(t)
	// Simulate approaching the upper bound without bypassing the SQL
	// monotonic guard. The real sequence can never be reset to zero.
	if _, err := handle.DB.ExecContext(ctx,
		"DROP TRIGGER stage_device_v2_connection_monotonic"); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx,
		"UPDATE stage_device_v2_connection_sequence SET generation=? WHERE singleton=1", int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AllocateV2ConnectionGeneration(ctx); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("exhausted sequence reused generation: %v", err)
	}
}

func TestV2GenerationUniqueAcrossOverlappingDatabaseConnections(t *testing.T) {
	ctx := context.Background()
	first, handle, _ := newRepository(t)
	root := filepath.Dir(filepath.Dir(handle.Path))
	secondHandle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer secondHandle.Close()
	second, err := deviceexperience.NewRepository(secondHandle.DB)
	if err != nil {
		t.Fatal(err)
	}
	// Independent SQLite connections emulate two overlapping Hub processes,
	// not merely two goroutines sharing one sql.DB. No allocated generation
	// can be repeated even if both runtimes read the same starting high-water.
	var wg sync.WaitGroup
	values := make(chan int64, 8)
	failures := make(chan error, 8)
	for _, repository := range []*deviceexperience.Repository{first, second} {
		wg.Add(1)
		go func(repo *deviceexperience.Repository) {
			defer wg.Done()
			for i := 0; i < 4; i++ {
				generation, err := repo.AllocateV2ConnectionGeneration(ctx)
				if err != nil {
					failures <- err
					return
				}
				values <- generation
			}
		}(repository)
	}
	wg.Wait()
	close(failures)
	close(values)
	for err := range failures {
		t.Fatalf("parallel Hub generation allocation failed: %v", err)
	}
	seen := map[int64]bool{}
	for generation := range values {
		if generation <= 0 || seen[generation] {
			t.Fatalf("parallel Hub reused generation=%d", generation)
		}
		seen[generation] = true
	}
	if len(seen) != 8 {
		t.Fatalf("only allocated %d/8 unique generations", len(seen))
	}
}
