package align

import "fmt"

func (o Options) validate() error {
	if err := o.Scoring.validate(); err != nil {
		return err
	}
	if o.MaxAlignments < 0 {
		return fmt.Errorf("max alignments must be >= 0")
	}
	if o.XDrop < 0 {
		return fmt.Errorf("x-drop must be >= 0")
	}
	if o.BandWidth < 0 {
		return fmt.Errorf("band width must be >= 0")
	}
	return nil
}

func (s Scoring) validate() error {
	if s.Match <= 0 {
		return fmt.Errorf("match score must be > 0")
	}
	if s.Mismatch >= 0 {
		return fmt.Errorf("mismatch score must be < 0")
	}
	if s.GapOpen >= 0 {
		return fmt.Errorf("gap open score must be < 0")
	}
	if s.GapExtend >= 0 {
		return fmt.Errorf("gap extend score must be < 0")
	}
	return nil
}
