"""Write insdc test fixtures: records as JSON, and Biopython's GenBank and EMBL.

Usage: python3 make_fixtures.py OUT_DIR

Builds Biopython SeqRecords that exercise the GenBank and EMBL writers, then
writes records.json (the records in the shape of insdc.Record, read by the Go
tests), and records.gb and records.embl written by Biopython's SeqIO.write.
"""
import json
import sys
import warnings
from pathlib import Path

from Bio import BiopythonWarning, SeqIO
from Bio.Seq import Seq
from Bio.SeqFeature import (AfterPosition, BeforePosition, BetweenPosition, CompoundLocation, ExactPosition,
                            OneOfPosition, Reference, SeqFeature, UnknownPosition, WithinPosition)

try:
    from Bio.SeqFeature import SimpleLocation
except ImportError:  # Biopython < 1.80
    from Bio.SeqFeature import FeatureLocation as SimpleLocation
from Bio.SeqRecord import SeqRecord

import record_json


def loc(start, end, strand=1, ref=None):
    return SimpleLocation(start, end, strand=strand, ref=ref)


def feat(ftype, location, **qualifiers):
    return SeqFeature(location, type=ftype, qualifiers=qualifiers)


def seq_of(n):
    return Seq(('ACGTNacgtRYKM' * (n // 13 + 1))[:n])


records = []

# 1. Features: locations of every kind and qualifier values that wrap.
features = [
    feat('source', loc(0, 300), mol_type=['genomic DNA']),
    feat('gene', loc(10, 100, -1), locus_tag=['TST_00001'], gene=['abcD']),
    feat('CDS', loc(10, 100, -1), locus_tag=['TST_00001'], codon_start=[1], transl_table=[11],
         product=['A very long product name that goes on and on beyond the eighty column limit of the format'],
         translation=['M' + 'ACDEFGHIKLMNPQRSTVWY' * 8],
         note=['He said "hello"', 'second note'], pseudo=[None], ribosomal_slippage=[True],
         score=[0.75], anticodon=['(pos:complement(20..22),aa:Ala,seq:tgc)'], empty=['']),
    feat('CDS', CompoundLocation([loc(110, 150), loc(160, 200)], operator='join'), product=['β-lactamase with a name long enough that it wraps onto a second line here']),
    feat('CDS', CompoundLocation([loc(160, 200, -1), loc(110, 150, -1)], operator='join'), product=['reverse join']),
    feat('misc_feature', CompoundLocation([loc(0, 5), loc(10, 15)], operator='order'), note=['order']),
    feat('misc_feature', SimpleLocation(BeforePosition(4), AfterPosition(20), strand=1), note=['partial']),
    feat('misc_feature', SimpleLocation(WithinPosition(4, left=4, right=8), BetweenPosition(30, left=29, right=30), strand=-1), note=['within/between']),
    feat('misc_feature', SimpleLocation(OneOfPosition(5, [ExactPosition(5), ExactPosition(7)]), ExactPosition(40)), note=['one-of']),
    feat('misc_feature', loc(41, 42), note=['single base']),
    feat('misc_feature', loc(50, 50), note=['between bases']),
    feat('misc_feature', loc(300, 300), note=['between the end and the start']),
    feat('misc_feature', loc(5, 25, ref='OTHER.1'), note=['on another record']),
    feat('misc_feature', SimpleLocation(UnknownPosition(), ExactPosition(60)), note=['unknown start']),
    feat('misc_feature', SimpleLocation(ExactPosition(60), UnknownPosition()), note=['unknown end']),
    feat('misc_feature', CompoundLocation([loc(i * 10, i * 10 + 5) for i in range(25)], operator='join'), note=['long location wraps at commas']),
    feat('repeat region', loc(0, 30), rpt_type=['direct'], rpt_unit_seq=['acgtacgt']),
    feat('regulatory_region_long_key', loc(0, 30), note=['key over 15 characters is cut at column 21']),
    feat('misc_feature', loc(70, 90), note=['x' * 150], inference=['a' + ' ' * 70 + 'b']),
]
r = SeqRecord(seq_of(300), id='TEST001.2', name='TEST001', description='Synthetic record one, with a description long enough to wrap onto more than one line in both formats',
              dbxrefs=['BioProject:PRJNA1', 'BioSample:SAMN1', 'Project:12'], features=features)
r.annotations.update({
    'molecule_type': 'DNA', 'topology': 'circular', 'data_file_division': 'BCT', 'date': '05-MAR-2024',
    'accessions': ['TEST001'], 'gi': '12345', 'keywords': ['alpha', 'beta gamma'], 'source': 'Testus syntheticus',
    'organism': 'Testus syntheticus', 'taxonomy': ['Bacteria', 'Testota', 'Testales'],
    'comment': 'First comment line\nSecond line that is long enough to need wrapping because it has very many words in it\n\nAfter a blank line',
})
ref1 = Reference()
ref1.location = [SimpleLocation(0, 300)]
ref1.authors = 'Smith,J., Jones,K. and a very long list of other authors who wrote the paper together'
ref1.title = 'A title'
ref1.journal = 'J. Test 1:1-10 (2024)'
ref1.pubmed_id = '123456'
ref1.consrtm = 'Test Consortium'
ref1.medline_id = '999'
ref1.comment = 'A remark'
ref2 = Reference()
ref2.journal = 'Unpublished'
r.annotations['references'] = [ref1, ref2]
records.append(r)

# 2. Defaults and header variations.
r = SeqRecord(seq_of(59), id='NOVERSION', name='<unknown name>', description='<unknown description>')
r.annotations.update({'molecule_type': 'DNA', 'data_file_division': 'PRO', 'date': '31-FEB-2024', 'source': None})
records.append(r)

r = SeqRecord(seq_of(60), id='LONGNAME', name='a_locus_name_longer_than_sixteen', description='long locus name')
r.annotations.update({'molecule_type': 'unassigned DNA', 'topology': 'linear', 'data_file_division': 'XYZ', 'keywords': [],
                      'organism': 'An organism name that is far longer than sixty-eight characters in total length',
                      'segment': '1 of 2', 'db_source': 'some source', 'structured_comment': {
                          'Genome-Assembly-Data': {'Assembly Method': 'Test v. 1', 'Coverage': '10x'},
                          'Genome-Annotation-Data': {'Annotation Provider': 'Bakta'}}})
records.append(r)

r = SeqRecord(seq_of(61), id='prot.x', name='PROT1', description='a protein')
r.annotations.update({'molecule_type': 'protein', 'date': 'bad date'})
records.append(r)

r = SeqRecord(seq_of(125), id='RNA1.1', name='RNA1', description='an RNA')
r.annotations.update({'molecule_type': 'mRNA', 'topology': 'a-very-long-topology', 'data_file_division': 'HUM',
                      'comment': ['list comment one', 'list comment two\n']})
records.append(r)

r = SeqRecord(Seq(''), id='EMPTY', name='EMPTY', description='empty sequence')
r.annotations.update({'molecule_type': 'DNA'})
records.append(r)

r = SeqRecord(seq_of(1234), id='abcdefghijklmnopqrstu', name='abcdefghijklmnopqrstu', description='long name and length')
r.annotations.update({'molecule_type': 'DNA'})
records.append(r)

out = Path(sys.argv[1])
out.mkdir(parents=True, exist_ok=True)
with warnings.catch_warnings():
    warnings.simplefilter('ignore', BiopythonWarning)
    SeqIO.write(records, out / 'records.gb', 'genbank')
    SeqIO.write(records, out / 'records.embl', 'embl')
(out / 'records.json').write_text(json.dumps([record_json.record_to_json(r) for r in records], indent=1, ensure_ascii=False) + '\n')
