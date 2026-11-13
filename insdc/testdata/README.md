# insdc test fixtures

`make_fixtures.py` builds Biopython records that exercise every part of
Biopython's GenBank and EMBL writers (locations of every kind, qualifier
wrapping and quoting, header defaults, comments, references, sequence blocks)
and writes:

- `records.json`: the records in the shape of `insdc.Record`, converted by
  `record_json.py`
- `records.gb` and `records.embl`: what Biopython's `SeqIO.write` writes for
  them

The Go tests write `records.json` with the `insdc` writers and compare with the
Biopython files byte for byte. To regenerate, with Biopython installed:

```
python3 -B make_fixtures.py .
```

The committed files were written with Biopython 1.88. `record_json.py` can
also convert records built elsewhere, e.g. by Bakta, for the same comparison.
