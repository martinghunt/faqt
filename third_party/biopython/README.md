# Biopython attribution

The GenBank and EMBL writers in `insdc/` (`insdc/genbank.go`,
`insdc/embl.go`, `insdc/writer.go` and `insdc/location.go`) are adapted from
Biopython 1.88 `Bio/SeqIO/InsdcIO.py` (the GenBank and EMBL writer classes and
the location string functions), so that their output matches Biopython's byte
for byte:

https://github.com/biopython/biopython/blob/biopython-188/Bio/SeqIO/InsdcIO.py

That file carries this notice:

    Copyright 2007-2016 by Peter Cock.  All rights reserved.

    This file is part of the Biopython distribution and governed by your
    choice of the "Biopython License Agreement" or the "BSD 3-Clause License".

faqt uses it under the BSD 3-Clause License. Biopython's license file, with
both licenses in full, is included here as `LICENSE.rst`. Retain this notice
when distributing the derived code. There is no runtime dependency on
Biopython; it is only used to write the test fixtures in `insdc/testdata`.
