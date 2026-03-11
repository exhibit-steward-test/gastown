//go:build !windows

package doltserver

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/steveyegge/gastown/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestCircuitBreakerConcurrentQueryPressure verifies that the Dolt connection
// pool handles concurrent query pressure gracefully without accumulating
// CLOSE_WAIT connections or causing cascading timeouts.
//
// This test addresses the production issue documented in gt-92jh where query
// storms caused circuit breaker trips and CLOSE_WAIT accumulation.
func TestCircuitBreakerConcurrentQueryPressure(t *testing.T) {
	port := testutil.StartIsolatedDoltContainer(t)

	// Create DSN with aggressive timeouts to simulate production pressure
	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%s)/gt_test?timeout=2s&readTimeout=5s&writeTimeout=5s", port)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err, "failed to open connection")
	defer db.Close()

	// Configure connection pool to simulate realistic multi-agent workload
	db.SetMaxOpenConns(10) // Limit total connections
	db.SetMaxIdleConns(5)  // Limit idle connections
	db.SetConnMaxLifetime(30 * time.Second)
	db.SetConnMaxIdleTime(10 * time.Second)

	// Create a test table to query
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS test_load (
			id INT PRIMARY KEY,
			payload VARCHAR(1000)
		)
	`)
	require.NoError(t, err, "failed to create test table")

	// Seed some data
	for i := 0; i < 100; i++ {
		_, err = db.ExecContext(ctx, "INSERT INTO test_load VALUES (?, ?)", i, fmt.Sprintf("payload_%d", i))
		require.NoError(t, err, "failed to seed data")
	}

	// Test 1: Verify basic query succeeds
	t.Run("baseline_query", func(t *testing.T) {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_load").Scan(&count)
		require.NoError(t, err, "baseline query should succeed")
		require.Equal(t, 100, count, "expected 100 rows")
	})

	// Test 2: Simulate concurrent query storm (20 parallel queries)
	t.Run("concurrent_query_storm", func(t *testing.T) {
		const numQueries = 20
		var wg sync.WaitGroup
		var successCount atomic.Int32
		var errorCount atomic.Int32

		for i := 0; i < numQueries; i++ {
			wg.Add(1)
			go func(queryID int) {
				defer wg.Done()

				queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				// Mix of read queries to create realistic pressure
				var count int
				err := db.QueryRowContext(queryCtx, "SELECT COUNT(*) FROM test_load WHERE id < ?", queryID*5).Scan(&count)
				if err != nil {
					t.Logf("Query %d failed: %v", queryID, err)
					errorCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}(i)
		}

		wg.Wait()

		// We expect most queries to succeed. Some may fail due to pool exhaustion,
		// but the system should not deadlock or accumulate zombie connections.
		t.Logf("Concurrent query results: %d succeeded, %d failed", successCount.Load(), errorCount.Load())
		require.Greater(t, int(successCount.Load()), numQueries/2,
			"expected at least 50%% of concurrent queries to succeed")
	})

	// Test 3: Verify connection recovery after pressure subsides
	t.Run("recovery_after_pressure", func(t *testing.T) {
		// Wait for connections to be released
		time.Sleep(2 * time.Second)

		// Verify database is still responsive
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_load").Scan(&count)
		require.NoError(t, err, "recovery query should succeed after pressure subsides")
		require.Equal(t, 100, count, "expected 100 rows after recovery")
	})

	// Test 4: Verify no connection leaks by checking pool stats
	t.Run("connection_pool_health", func(t *testing.T) {
		stats := db.Stats()
		t.Logf("Connection pool stats: Open=%d, InUse=%d, Idle=%d, WaitCount=%d",
			stats.OpenConnections, stats.InUse, stats.Idle, stats.WaitCount)

		// After settling, we should have few or no connections in use
		require.LessOrEqual(t, stats.InUse, 2,
			"expected minimal connections in use after test completes")

		// Idle connections should be within configured limit
		require.LessOrEqual(t, stats.Idle, 5,
			"idle connections should not exceed MaxIdleConns")
	})
}

// TestCircuitBreakerSlowQueryTimeout verifies that slow queries timeout
// properly and don't block the connection pool indefinitely.
//
// This addresses the CLOSE_WAIT accumulation issue where Dolt's 8-hour
// default timeout could leave connections zombie'd.
func TestCircuitBreakerSlowQueryTimeout(t *testing.T) {
	port := testutil.StartIsolatedDoltContainer(t)

	// Create DSN with very aggressive read timeout
	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%s)/gt_test?timeout=2s&readTimeout=3s&writeTimeout=3s", port)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err, "failed to open connection")
	defer db.Close()

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	ctx := context.Background()

	// Create a table for slow query simulation
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS slow_test (
			id INT PRIMARY KEY,
			data VARCHAR(100)
		)
	`)
	require.NoError(t, err, "failed to create slow_test table")

	// Seed enough data to make queries measurable
	for i := 0; i < 1000; i++ {
		_, err = db.ExecContext(ctx, "INSERT INTO slow_test VALUES (?, ?)", i, fmt.Sprintf("data_%d", i))
		require.NoError(t, err, "failed to seed data")
	}

	t.Run("query_timeout_enforcement", func(t *testing.T) {
		// Create a context with tight timeout
		queryCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		// Attempt a potentially slow query with a tight timeout
		start := time.Now()
		rows, err := db.QueryContext(queryCtx, `
			SELECT s1.id, s2.id, s3.id
			FROM slow_test s1
			CROSS JOIN slow_test s2
			CROSS JOIN slow_test s3
			LIMIT 100
		`)
		elapsed := time.Since(start)

		if rows != nil {
			rows.Close()
		}

		// We expect this to either succeed quickly or timeout
		if err != nil {
			t.Logf("Query timed out as expected: %v (elapsed: %v)", err, elapsed)
			// Verify timeout happened within reasonable bounds (not 8 hours!)
			require.Less(t, elapsed, 10*time.Second,
				"timeout should fire within seconds, not minutes/hours")
		} else {
			t.Logf("Query completed in %v", elapsed)
		}
	})

	t.Run("pool_remains_healthy_after_timeout", func(t *testing.T) {
		// After timeout, pool should still work
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM slow_test").Scan(&count)
		require.NoError(t, err, "pool should be healthy after timeout")
		require.Equal(t, 1000, count, "expected all rows present")
	})
}

// TestCircuitBreakerConnectionLifecycle verifies that connections are
// properly cleaned up and don't accumulate in CLOSE_WAIT state.
func TestCircuitBreakerConnectionLifecycle(t *testing.T) {
	port := testutil.StartIsolatedDoltContainer(t)

	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%s)/gt_test?timeout=2s&readTimeout=5s&writeTimeout=5s", port)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err, "failed to open connection")
	defer db.Close()

	// Set very short connection lifetimes to force recycling
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(2 * time.Second) // Force frequent recycling
	db.SetConnMaxIdleTime(1 * time.Second)

	ctx := context.Background()

	// Create test table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS lifecycle_test (
			id INT PRIMARY KEY,
			value INT
		)
	`)
	require.NoError(t, err, "failed to create table")

	t.Run("connection_recycling", func(t *testing.T) {
		// Perform queries over 6 seconds, forcing at least one full connection lifecycle
		for i := 0; i < 10; i++ {
			var result int
			err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
			require.NoError(t, err, "query %d should succeed during recycling", i)
			time.Sleep(700 * time.Millisecond)
		}

		stats := db.Stats()
		t.Logf("After recycling: Open=%d, InUse=%d, Idle=%d, WaitCount=%d, WaitDuration=%v",
			stats.OpenConnections, stats.InUse, stats.Idle, stats.WaitCount, stats.WaitDuration)

		// Connections should have been recycled, but pool should remain healthy
		require.LessOrEqual(t, stats.OpenConnections, 3,
			"open connections should not exceed max")
	})

	t.Run("no_connection_leaks", func(t *testing.T) {
		// Force all connections to close by waiting past idle timeout
		time.Sleep(2 * time.Second)

		// Open and close a fresh connection to verify pool is clean
		var result int
		err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
		require.NoError(t, err, "new connection should work after full idle timeout")

		stats := db.Stats()
		t.Logf("Final pool stats: Open=%d, InUse=%d, Idle=%d",
			stats.OpenConnections, stats.InUse, stats.Idle)

		// After settling, we should have minimal open connections
		require.LessOrEqual(t, stats.OpenConnections, 2,
			"should have minimal connections after idle period")
	})
}

// TestCircuitBreakerN1QueryPattern verifies that N+1 query patterns
// (like the ones patrolled in PR #2285) can be detected via connection metrics.
//
// This test doesn't prevent N+1 queries, but it demonstrates how connection
// pool pressure increases with query fan-out, which can be monitored.
func TestCircuitBreakerN1QueryPattern(t *testing.T) {
	port := testutil.StartIsolatedDoltContainer(t)

	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%s)/gt_test?timeout=2s&readTimeout=5s&writeTimeout=5s", port)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err, "failed to open connection")
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)

	ctx := context.Background()

	// Setup: Create parent and child tables
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS n1_parent (
			id INT PRIMARY KEY,
			name VARCHAR(50)
		)
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS n1_child (
			id INT PRIMARY KEY,
			parent_id INT,
			value VARCHAR(50)
		)
	`)
	require.NoError(t, err)

	// Seed parent records
	for i := 0; i < 20; i++ {
		_, err = db.ExecContext(ctx, "INSERT INTO n1_parent VALUES (?, ?)", i, fmt.Sprintf("parent_%d", i))
		require.NoError(t, err)
	}

	// Seed child records (multiple per parent)
	for i := 0; i < 20; i++ {
		for j := 0; j < 5; j++ {
			childID := i*5 + j
			_, err = db.ExecContext(ctx, "INSERT INTO n1_child VALUES (?, ?, ?)", childID, i, fmt.Sprintf("child_%d", childID))
			require.NoError(t, err)
		}
	}

	t.Run("n1_query_fan_out_detection", func(t *testing.T) {
		// Simulate N+1 query pattern (1 query for parents + N queries for children)
		startStats := db.Stats()

		// Initial query for parents
		rows, err := db.QueryContext(ctx, "SELECT id, name FROM n1_parent ORDER BY id")
		require.NoError(t, err)
		defer rows.Close()

		var queryCount int
		for rows.Next() {
			var id int
			var name string
			require.NoError(t, rows.Scan(&id, &name))

			// N+1 anti-pattern: separate query per parent
			var childCount int
			err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM n1_child WHERE parent_id = ?", id).Scan(&childCount)
			require.NoError(t, err)
			queryCount++
		}

		endStats := db.Stats()

		t.Logf("N+1 pattern executed %d child queries", queryCount)
		t.Logf("Pool pressure - WaitCount: %d -> %d, MaxOpenConnections: %d",
			startStats.WaitCount, endStats.WaitCount, endStats.MaxOpenConnections)

		// We executed 20+ queries - if this caused connection pool waits,
		// that's a signal of query pressure
		if endStats.WaitCount > startStats.WaitCount {
			t.Logf("WARNING: N+1 query pattern caused connection pool waits (delta: %d)",
				endStats.WaitCount-startStats.WaitCount)
		}
	})

	t.Run("optimized_join_pattern", func(t *testing.T) {
		// Demonstrate the better pattern: single JOIN query
		startStats := db.Stats()

		rows, err := db.QueryContext(ctx, `
			SELECT p.id, p.name, COUNT(c.id) as child_count
			FROM n1_parent p
			LEFT JOIN n1_child c ON p.id = c.parent_id
			GROUP BY p.id, p.name
			ORDER BY p.id
		`)
		require.NoError(t, err)
		defer rows.Close()

		var resultCount int
		for rows.Next() {
			var id int
			var name string
			var childCount int
			require.NoError(t, rows.Scan(&id, &name, &childCount))
			resultCount++
		}

		endStats := db.Stats()

		require.Equal(t, 20, resultCount, "should get all parent results")
		t.Logf("Optimized pattern - WaitCount: %d -> %d",
			startStats.WaitCount, endStats.WaitCount)

		// Single query should cause no additional pool pressure
		require.Equal(t, startStats.WaitCount, endStats.WaitCount,
			"optimized query should not increase pool wait count")
	})
}
