package align

// containmentIndex indexes points on retained alignment paths. A possible
// subpath must begin on a paired-base diagonal, an insertion at a fixed B
// coordinate, or a deletion at a fixed A coordinate. The interval lookup is only
// a candidate filter: pathContains remains the final authority on redundancy.
type containmentIndex struct {
	groups  map[containmentGroup]map[containmentLine]int
	nodes   []containmentNode
	matches []Match
}
type containmentGroup struct {
	a, b    int
	reverse bool
}
type containmentLine struct {
	coordinate int
	kind       byte
}
type containmentNode struct {
	lo, hi, maxHi      int
	left, right, match int
	priority           uint64
}

func (x *containmentIndex) add(m Match) {
	if x.groups == nil {
		x.groups = make(map[containmentGroup]map[containmentLine]int)
		x.nodes = append(x.nodes, containmentNode{maxHi: -int(^uint(0)>>1) - 1})
	}
	key := containmentGroup{m.AContig, m.BContig, m.Reverse}
	lines := x.groups[key]
	if lines == nil {
		lines = make(map[containmentLine]int)
		x.groups[key] = lines
	}
	id := len(x.matches)
	x.matches = append(x.matches, m)
	q, t := m.A.Start, m.B.Start
	if m.Reverse {
		t = -m.B.End
	}
	for i := 0; i < len(m.Ops); i++ {
		op := m.Ops[i]
		switch op.Code {
		case '=', 'X':
			lo, diag := q, t-q
			q += op.Length
			t += op.Length
			// Substitutions do not change a paired-base diagonal. CIGAR
			// equality is checked later, so coalesce these index intervals.
			for i+1 < len(m.Ops) && (m.Ops[i+1].Code == '=' || m.Ops[i+1].Code == 'X') {
				i++
				q += m.Ops[i].Length
				t += m.Ops[i].Length
			}
			x.insertLine(lines, containmentLine{diag, 'P'}, lo, q, id)
		case 'I':
			x.insertLine(lines, containmentLine{t, 'I'}, q, q+op.Length, id)
			q += op.Length
		case 'D':
			x.insertLine(lines, containmentLine{q, 'D'}, t, t+op.Length, id)
			t += op.Length
		}
	}
}
func (x *containmentIndex) insertLine(lines map[containmentLine]int, key containmentLine, lo, hi, id int) {
	n := len(x.nodes)
	// Deterministic SplitMix64 priorities avoid global random state and keep
	// ordinary monotonically ordered genomic coordinates balanced.
	z := uint64(n) + 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	x.nodes = append(x.nodes, containmentNode{lo: lo, hi: hi, maxHi: hi, match: id, priority: z})
	lines[key] = x.insert(lines[key], n)
}
func (x *containmentIndex) update(n int) {
	a := &x.nodes[n]
	a.maxHi = max(a.hi, x.nodes[a.left].maxHi, x.nodes[a.right].maxHi)
}
func (x *containmentIndex) insert(root, n int) int {
	if root == 0 {
		return n
	}
	if x.nodes[n].lo < x.nodes[root].lo || (x.nodes[n].lo == x.nodes[root].lo && n < root) {
		left := x.insert(x.nodes[root].left, n)
		x.nodes[root].left = left
		if x.nodes[left].priority < x.nodes[root].priority {
			x.nodes[root].left = x.nodes[left].right
			x.nodes[left].right = root
			x.update(root)
			root = left
		}
	} else {
		right := x.insert(x.nodes[root].right, n)
		x.nodes[root].right = right
		if x.nodes[right].priority < x.nodes[root].priority {
			x.nodes[root].right = x.nodes[right].left
			x.nodes[right].left = root
			x.update(root)
			root = right
		}
	}
	x.update(root)
	return root
}
func (x *containmentIndex) contains(m Match) bool {
	lines := x.groups[containmentGroup{m.AContig, m.BContig, m.Reverse}]
	if lines == nil {
		return false
	}
	q, t := m.A.Start, m.B.Start
	if m.Reverse {
		t = -m.B.End
	}
	return x.query(lines[containmentLine{t - q, 'P'}], q, m) || x.query(lines[containmentLine{t, 'I'}], q, m) || x.query(lines[containmentLine{q, 'D'}], t, m)
}
func (x *containmentIndex) query(root, point int, m Match) bool {
	if root == 0 || x.nodes[root].maxHi < point {
		return false
	}
	n := &x.nodes[root]
	if n.lo > point {
		return x.query(n.left, point, m)
	}
	if n.hi >= point && pathContains(x.matches[n.match], m) {
		return true
	}
	return x.query(n.left, point, m) || x.query(n.right, point, m)
}
