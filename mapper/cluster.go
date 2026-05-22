package mapper

import (
	"fmt"
	"slices"

	"github.com/martinghunt/faqt/minimizer"
)

func (c DiagonalClusterer) Cluster(anchors []minimizer.Anchor) ([]Cluster, error) {
	if err := c.Options.validate(); err != nil {
		return nil, err
	}
	if len(anchors) == 0 {
		return nil, nil
	}

	sorted := append([]minimizer.Anchor(nil), anchors...)
	slices.SortFunc(sorted, compareAnchors)

	clusters := make([]Cluster, 0, len(sorted))
	start := 0
	for i := 1; i <= len(sorted); i++ {
		if i == len(sorted) || !sameClusterKey(sorted[start], sorted[i], c.Options.DiagonalBand) {
			if i-start >= c.Options.MinAnchors {
				clusters = append(clusters, makeCluster(sorted[start:i], c.Options.DiagonalBand))
			}
			start = i
		}
	}
	return clusters, nil
}

func (o ClusterOptions) validate() error {
	if o.DiagonalBand <= 0 {
		return fmt.Errorf("diagonal band must be > 0")
	}
	if o.MinAnchors <= 0 {
		return fmt.Errorf("minimum anchors must be > 0")
	}
	return nil
}

func compareAnchors(a, b minimizer.Anchor) int {
	if a.RefID != b.RefID {
		return cmpInt(a.RefID, b.RefID)
	}
	if relStrand(a) != relStrand(b) {
		return cmpInt(int(relStrand(a)), int(relStrand(b)))
	}
	da := diagonalValue(a)
	db := diagonalValue(b)
	if da != db {
		return cmpInt(da, db)
	}
	if a.QueryPos != b.QueryPos {
		return cmpInt(a.QueryPos, b.QueryPos)
	}
	if relStrand(a) == 0 {
		return cmpInt(a.RefPos, b.RefPos)
	}
	return cmpInt(b.RefPos, a.RefPos)
}

func sameClusterKey(a, b minimizer.Anchor, diagonalBand int) bool {
	return a.RefID == b.RefID &&
		relStrand(a) == relStrand(b) &&
		diagonalBucket(a, diagonalBand) == diagonalBucket(b, diagonalBand)
}

func makeCluster(anchors []minimizer.Anchor, diagonalBand int) Cluster {
	out := Cluster{
		RefID:          anchors[0].RefID,
		RelativeStrand: relStrand(anchors[0]),
		Band:           diagonalBucket(anchors[0], diagonalBand),
		Anchors:        append([]minimizer.Anchor(nil), anchors...),
		QueryStart:     anchors[0].QueryPos,
		QueryEnd:       anchors[0].QueryPos,
		RefStart:       anchors[0].RefPos,
		RefEnd:         anchors[0].RefPos,
	}
	for _, anchor := range anchors[1:] {
		if anchor.QueryPos < out.QueryStart {
			out.QueryStart = anchor.QueryPos
		}
		if anchor.QueryPos > out.QueryEnd {
			out.QueryEnd = anchor.QueryPos
		}
		if anchor.RefPos < out.RefStart {
			out.RefStart = anchor.RefPos
		}
		if anchor.RefPos > out.RefEnd {
			out.RefEnd = anchor.RefPos
		}
	}
	return out
}

func relStrand(anchor minimizer.Anchor) uint8 {
	return anchor.QueryStrand ^ anchor.RefStrand
}

func diagonalValue(anchor minimizer.Anchor) int {
	if relStrand(anchor) == 0 {
		return anchor.QueryPos - anchor.RefPos
	}
	return anchor.QueryPos + anchor.RefPos
}

func diagonalBucket(anchor minimizer.Anchor, diagonalBand int) int {
	return diagonalValue(anchor) / diagonalBand
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
