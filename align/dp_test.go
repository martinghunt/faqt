package align

import "testing"

func TestSuffixAlignClipping(t *testing.T) {
	scoring := DefaultAligner().Options.Scoring
	tests := []struct {
		name          string
		query         string
		ref           string
		wantScore     int
		wantQueryClip int
		wantRefClip   int
	}{
		{
			name:          "matching suffix",
			query:         "TTACGT",
			ref:           "GGACGT",
			wantScore:     8,
			wantQueryClip: 2,
			wantRefClip:   2,
		},
		{
			name:          "no local match",
			query:         "AAAA",
			ref:           "CCCC",
			wantScore:     0,
			wantQueryClip: 4,
			wantRefClip:   4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := suffixAlign([]byte(tt.query), []byte(tt.ref), scoring, 0)
			if err != nil {
				t.Fatalf("suffixAlign() error = %v", err)
			}
			if got.score != tt.wantScore || got.queryClip != tt.wantQueryClip || got.refClip != tt.wantRefClip {
				t.Fatalf("suffixAlign() = score %d, query clip %d, ref clip %d; want %d, %d, %d",
					got.score, got.queryClip, got.refClip, tt.wantScore, tt.wantQueryClip, tt.wantRefClip)
			}
		})
	}
}
