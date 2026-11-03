package provider

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A careful provider (Provider.CapCareful, plugins#349) holds an account
// whose usage it can't tell and has it read again (ReadUsageNow). Claude
// Code's /usage runs only when claudeWindows' own rules say, so that ask
// read nothing: after a restart every saved account kept its reading from
// disk, which counts as failed, and the signed-in one whose /usage failed
// waited out its 5 to 15 minutes. Every account held, nothing passed the
// gateway until a card's refresh or another restart.
func TestCarefulReadsClaudeAccounts(t *testing.T) {
	claudeThree(t)
	var text atomic.Value
	text.Store("Current session: 17% used\nCurrent week (all models): 38% used")
	fakeClaudeUsage(t, &text, nil)
	claudeUsageWait = func() time.Duration { return 10 * time.Minute }
	var active string
	var saved []string
	for _, l := range Logins("claude") {
		if l.Active {
			active = l.User
		} else {
			saved = append(saved, l.User)
		}
	}
	if active == "" || len(saved) == 0 {
		t.Fatalf("active %q, saved %v", active, saved)
	}
	var mu sync.Mutex
	runs := map[string]int{} // config dir ("" for Claude Code's own) → runs
	var activeFails atomic.Bool
	var holdIn atomic.Value // a config dir whose /usage runs till release
	holdIn.Store("")
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var freed sync.Once
	free := func() { freed.Do(func() { close(release) }) }
	UsageClaudeVia(func(_ context.Context, dir string) (string, error) {
		mu.Lock()
		runs[dir]++
		mu.Unlock()
		if dir != "" && dir == holdIn.Load().(string) {
			entered <- struct{}{}
			<-release
		}
		if dir == "" && activeFails.Load() {
			return "", errors.New("/usage couldn't load the plan's usage")
		}
		return text.Load().(string), nil
	})
	ranIn := func(user string) int {
		mu.Lock()
		defer mu.Unlock()
		if user == active {
			return runs[""]
		}
		return runs[claudeAccountDir(user)]
	}
	// a fresh process: nothing in memory, the readings kept on disk
	restart := func() {
		claudeUsage.Lock()
		claudeUsage.m = nil
		claudeUsage.Unlock()
		loginUsageCache.Lock()
		loginUsageCache.m, loginUsageCache.pending = nil, nil
		loginUsageCache.Unlock()
		usedCache.Lock()
		usedCache.m, usedCache.at, usedCache.loading = map[string]map[string]Allowance{}, map[string]time.Time{}, map[string]chan struct{}{}
		usedCache.stale, usedCache.read = nil, nil
		usedCache.Unlock()
		lastQuotas.Lock()
		lastQuotas.m, lastQuotas.loaded = nil, false
		lastQuotas.Unlock()
		readAsked.Lock()
		readAsked.m = nil
		readAsked.Unlock()
	}
	t.Cleanup(restart)
	readNow := func(users ...string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ReadUsageNow(ctx, "claude", users...)
	}
	readWell := func(step, user string) {
		t.Helper()
		if at, failed, ok := UsageRead("claude", user); !ok || failed || at.IsZero() {
			t.Fatalf("%s: %s reads ok=%v failed=%v at=%v", step, user, ok, failed, at)
		}
	}
	// back by d: the floor and the wait are counted from tried
	backBy := func(user string, d time.Duration) {
		claudeUsage.Lock()
		e := claudeUsage.m[strings.ToLower(user)]
		e.tried = e.tried.Add(-d)
		claudeUsage.m[strings.ToLower(user)] = e
		claudeUsage.Unlock()
	}

	// every card read once, kept on disk; then magpie restarts
	RefreshUsage(context.Background(), "claude", "")
	restart()

	// what nobody careful asked for still keeps to what was kept: the
	// Usage page opened starts no Claude Code per saved account
	LoginUsage(context.Background(), "claude")
	for _, u := range saved {
		if n := ranIn(u); n != 1 {
			t.Fatalf("a read nobody asked for ran %s's /usage: %d runs", u, n)
		}
	}

	// careful's ask after the restart: each account read, once
	readNow(append([]string{active}, saved...)...)
	for _, u := range saved {
		if n := ranIn(u); n != 2 {
			t.Fatalf("after a restart careful's ask ran %s's /usage %d times, want once", u, n-1)
		}
		readWell("after a restart", u)
	}
	readWell("after a restart", active)

	// asked again while that read runs (careful asks on every request it
	// holds, and /usage takes 5 to 20s): one run, a look meanwhile starts
	// no second one, and once it lands well nothing is read again for the
	// ask made while it ran, past the floor too
	restart()
	u := saved[0]
	before := ranIn(u)
	holdIn.Store(claudeAccountDir(u))
	done := make(chan struct{})
	go func() {
		defer close(done)
		readNow(u)
	}()
	t.Cleanup(func() { free(); <-done })
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatalf("careful's ask never ran %s's /usage", u)
	}
	holdIn.Store("")
	gaveUp, cancel := context.WithCancel(context.Background())
	cancel()
	ReadUsageNow(gaveUp, "claude", u) // the next request's ask, its wait given up
	claudeWindows(context.Background(), u, false)
	if n := ranIn(u); n != before+1 {
		t.Fatalf("a look while %s's /usage ran started another: %d runs", u, n-before)
	}
	free()
	<-done
	readWell("after the read it waited on", u)
	backBy(u, claudeAskFloor+time.Second)
	claudeWindows(context.Background(), u, false) // routing's next look, a minute on
	if n := ranIn(u); n != before+1 {
		t.Fatalf("read well, %s's /usage ran again for an ask made while it ran: %d runs", u, n-before)
	}

	// the signed-in account's /usage fails: careful asks again, and it is
	// read again once its wait is up, not on every ask, though Claude Code
	// wasn't used since (every request held, nothing went through it)
	restart()
	claudeUsedSince = func(time.Time) bool { return false }
	activeFails.Store(true)
	readNow(active)
	if _, failed, _ := UsageRead("claude", active); !failed {
		t.Fatal("a failed /usage read as good")
	}
	activeFails.Store(false)
	before = ranIn(active)
	backBy(active, claudeAskFloor+time.Second)
	readNow(active)
	if n := ranIn(active); n != before {
		t.Fatalf("a failing account ran /usage again before its wait: %d runs", n-before)
	}
	backBy(active, 10*time.Minute)
	readNow(active)
	if n := ranIn(active); n != before+1 {
		t.Fatalf("careful's ask after the wait ran the signed-in account's /usage %d times, want once", n-before)
	}
	readWell("after its wait", active)
}
