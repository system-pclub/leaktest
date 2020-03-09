//Package leaktest is copied and modified from the leaktest package in 
//cockroachdb


// Package leaktest provides tools to detect leaked goroutines in tests.
// To use it, call "defer leaktest.AfterTest(t)()" at the beginning of each
// test that may use goroutines.
package leaktest

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
	"github.com/petermattis/goid"
)

// interestingGoroutines returns all goroutines we care about for the purpose
// of leak checking. It excludes testing or runtime ones.
func interestingGoroutines() map[int64]string {
	buf := make([]byte, 2<<20)
	buf = buf[:runtime.Stack(buf, true)]
	gs := make(map[int64]string)
	for _, g := range strings.Split(string(buf), "\n\n") {
		sl := strings.SplitN(g, "\n", 2)
		if len(sl) != 2 {
			continue
		}
		stack := strings.TrimSpace(sl[1])
		if strings.HasPrefix(stack, "testing.RunTests") {
			continue
		}

		if stack == "" ||
			// Ignore HTTP keep alives
			strings.Contains(stack, ").readLoop(") ||
			strings.Contains(stack, ").writeLoop(") ||
			// Ignore the raven client, which is created lazily on first use.
			strings.Contains(stack, "raven-go.(*Client).Capture") ||
			// Seems to be gccgo specific.
			(runtime.Compiler == "gccgo" && strings.Contains(stack, "testing.T.Parallel")) ||
			// Below are the stacks ignored by the upstream leaktest code.
			strings.Contains(stack, "testing.Main(") ||
			strings.Contains(stack, "testing.tRunner(") ||
			strings.Contains(stack, "runtime.goexit") ||
			strings.Contains(stack, "created by runtime.gc") ||
			strings.Contains(stack, "interestingGoroutines") ||
			strings.Contains(stack, "runtime.MHeap_Scavenger") ||
			strings.Contains(stack, "signal.signal_recv") ||
			strings.Contains(stack, "sigterm.handler") ||
			strings.Contains(stack, "runtime_mcall") ||
			strings.Contains(stack, "goroutine in C code") ||
			strings.Contains(stack, "runtime.CPUProfile") {
			continue
		}
		gs[goid.ExtractGID([]byte(g))] = g
	}

	return gs
}

// diffGoroutines compares the current goroutines with the base snapshort and
// returns an error if they differ.
func diffGoroutines(base map[int64]string) error {
	var leaked []string
	for id, stack := range interestingGoroutines() {
		if _, ok := base[id]; !ok {
			leaked = append(leaked, stack)
		}
	}
	if len(leaked) == 0 {
		return nil
	}

	sort.Strings(leaked)
	var b strings.Builder
	for _, g := range leaked {
		b.WriteString(fmt.Sprintf("Leaked goroutine: %v\n\n", g))
	}
	return fmt.Errorf(b.String())
}


func AfterTest(t testing.TB) func() {
	orig := interestingGoroutines()

	return func() {
		_ = recover()
		deadline := timeutil.Now().Add(5 * time.Second)
		for {
			if err := diffGoroutines(orig); err != nil {
				if timeutil.Now().Before(deadline) {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				t.Error(err)
			}
			break
		}

	}
}