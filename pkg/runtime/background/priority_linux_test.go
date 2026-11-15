package background

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func niceOf(t *testing.T, tid int) int {
	t.Helper()
	b, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(tid) + "/stat")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+2:]))
	n, err := strconv.Atoi(fields[16])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBackgroundThreadsRunAtTheirNice(t *testing.T) {
	p := New(1, 7)
	defer p.Close()
	var ran, helped int
	p.Run(func() { ran = niceOf(t, unix.Gettid()) })
	p.Split(2, func(lo, _ int) {
		if lo == 1 {
			helped = niceOf(t, unix.Gettid())
		}
	})
	if ran != 7 || helped != 7 {
		t.Errorf("nice: Run %d, helper %d, want 7", ran, helped)
	}
}
