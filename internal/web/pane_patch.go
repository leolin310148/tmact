package web

// panePatchShiftParam is the query parameter a pane-stream consumer sets to
// opt into "drop" patches. Consumers that predate it (a cached browser bundle,
// an older peer) never send it and keep receiving plain prefix patches.
const panePatchShiftParam = "shift"

// linePatch returns the smallest patch turning prev into next: the client
// drops the first drop lines, keeps the next from lines, and appends tail.
//
// capture-pane -S -N is a sliding window, so once a pane's history exceeds N
// every new output line shifts the whole capture up and the plain common
// prefix collapses to zero. With allowShift the scan also tries aligning next
// against prev[k:], which turns that full resend into drop=k plus the changed
// bottom rows. Any k is correct because prev[k:k+from] == next[:from] holds by
// construction; the search only picks the cheapest one.
func linePatch(prev, next []string, allowShift bool) (drop, from int, tail []string) {
	from = commonPrefix(prev, next)
	if allowShift && len(next) > 0 {
		best := from
		for k := 1; k < len(prev) && len(prev)-k > best; k++ {
			if prev[k] != next[0] {
				continue
			}
			if p := commonPrefix(prev[k:], next); p > best {
				drop, best = k, p
			}
		}
		from = best
	}
	return drop, from, append([]string(nil), next[from:]...)
}

func commonPrefix(a, b []string) int {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	return p
}
