package insdc

import (
	"fmt"
	"strconv"
	"strings"
)

// locationString is Biopython's _insdc_location_string.
func locationString(l Location, recordLength int) (string, error) {
	if len(l.Parts) != 0 {
		if l.strand() == -1 {
			// complement outside the join, parts in reverse order.
			parts := make([]string, len(l.Parts))
			for i := range l.Parts {
				part, err := locationStringIgnoringStrand(l.Parts[len(l.Parts)-1-i], recordLength)
				if err != nil {
					return "", err
				}
				parts[i] = part
			}
			return "complement(" + l.Operator + "(" + strings.Join(parts, ",") + "))", nil
		}
		parts := make([]string, len(l.Parts))
		for i, p := range l.Parts {
			part, err := locationString(p, recordLength)
			if err != nil {
				return "", err
			}
			parts[i] = part
		}
		return l.Operator + "(" + strings.Join(parts, ",") + ")", nil
	}
	loc, err := locationStringIgnoringStrand(l, recordLength)
	if err != nil {
		return "", err
	}
	if l.Strand == -1 {
		return "complement(" + loc + ")", nil
	}
	return loc, nil
}

// locationStringIgnoringStrand is Biopython's
// _insdc_location_string_ignoring_strand_and_subfeatures.
func locationStringIgnoringStrand(l Location, recordLength int) (string, error) {
	ref := ""
	if l.Ref != "" {
		ref = l.Ref + ":"
	}
	start, end := l.Start, l.End
	if start.Kind == Exact && end.Kind == Exact && start.Value == end.Value {
		// A zero-length slice: the point between two letters.
		if end.Value == recordLength {
			return fmt.Sprintf("%s%d^1", ref, recordLength), nil
		}
		return fmt.Sprintf("%s%d^%d", ref, end.Value, end.Value+1), nil
	}
	if start.Kind == Exact && end.Kind == Exact && start.Value+1 == end.Value {
		return fmt.Sprintf("%s%d", ref, end.Value), nil
	}
	if start.Kind == Unknown || end.Kind == Unknown {
		if start.Kind == Unknown && end.Kind == Unknown {
			return "", fmt.Errorf("feature with unknown location")
		}
		if start.Kind == Unknown {
			endString, err := positionString(end, 0)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s<%d..%s", ref, end.Value, endString), nil
		}
		startString, err := positionString(start, 1)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s%s..>%d", ref, startString, start.Value+1), nil
	}
	startString, err := positionString(start, 1)
	if err != nil {
		return "", err
	}
	endString, err := positionString(end, 0)
	if err != nil {
		return "", err
	}
	return ref + startString + ".." + endString, nil
}

// positionString is Biopython's _insdc_feature_position_string. offset 1
// converts a 0-based start to 1-based.
func positionString(p Position, offset int) (string, error) {
	switch p.Kind {
	case Exact:
		return strconv.Itoa(p.Value + offset), nil
	case Within:
		return fmt.Sprintf("(%d.%d)", p.Left+offset, p.Right+offset), nil
	case Between:
		return fmt.Sprintf("(%d^%d)", p.Left+offset, p.Right+offset), nil
	case Before:
		return fmt.Sprintf("<%d", p.Value+offset), nil
	case After:
		return fmt.Sprintf(">%d", p.Value+offset), nil
	case OneOf:
		choices := make([]string, len(p.Choices))
		for i, choice := range p.Choices {
			s, err := positionString(choice, offset)
			if err != nil {
				return "", err
			}
			choices[i] = s
		}
		return "one-of(" + strings.Join(choices, ",") + ")", nil
	default:
		return "", fmt.Errorf("unsupported position kind %d", p.Kind)
	}
}
