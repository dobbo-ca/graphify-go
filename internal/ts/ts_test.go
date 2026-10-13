package ts

import "testing"

func TestLimit(t *testing.T) {
	defer func(b uint32) { Budget = b }(Budget)
	Budget = 65536
	for _, c := range []struct {
		n    int
		want uint32
	}{{0, 65536}, {4096, 65536}, {8192, 131072}, {16384, 262144}, {32768, 131072}, {65536, 65536}, {9 << 20, 65536}} {
		if got := limit(c.n); got != c.want {
			t.Errorf("limit(%d) = %d, want %d", c.n, got, c.want)
		}
	}
	Budget = 1<<32 - 1
	if got := limit(16384); got != 1<<32-1 {
		t.Errorf("limit(16384) at the largest budget = %d", got)
	}
	Budget = 0
	if got := limit(16384); got != 0 {
		t.Errorf("limit(16384) with no budget = %d, want 0", got)
	}
}
