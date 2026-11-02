import tempfile
from pathlib import Path
import unittest
from verify import read_fasta,replay

class TestReplay(unittest.TestCase):
    def test_normalization_and_duplicate_names(self):
        with tempfile.TemporaryDirectory() as tmp:
            p=Path(tmp)/'x.fa';p.write_text('>a desc\nacguRN\n>b\nAAA\n')
            self.assertEqual(read_fasta(p),{'a':'ACGTNN','b':'AAA'})
            p.write_text('>a\nACG\n>a\nTTT\n')
            with self.assertRaises(ValueError):read_fasta(p)
    def test_reverse_indel_paf(self):
        # ACGTT reverse-complements to AACGT; B has an inserted C.
        with tempfile.TemporaryDirectory() as tmp:
            p=Path(tmp)/'x.paf';p.write_text('a\t5\t0\t5\t-\tb\t6\t0\t6\t5\t6\t255\tcg:Z:2=1D3=\n')
            self.assertEqual(replay(p,{'a':'ACGTT'},{'b':'AACCGT'}),1)
            with self.assertRaises(ValueError):replay(p,{'a':'ACGTT'},{'b':'TACCGT'})

if __name__=='__main__':unittest.main()
