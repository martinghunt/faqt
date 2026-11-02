import unittest
from analyze import normalize,merge,intersect,Placements
from tempfile import TemporaryDirectory
from pathlib import Path

def paf(q,qs,qe,t,ts,te,strand,cigar):
    return [q,str(max(100,qe)),str(qs),str(qe),strand,t,str(max(100,te)),str(ts),str(te),'1','1','255','cg:Z:'+cigar]

class TestPlacements(unittest.TestCase):
    def test_swapped_forward_indels(self):
        a=normalize(paf('a',10,20,'b',30,41,'+','2=1I3=2D4='),0)
        b=normalize(paf('b',30,41,'a',10,20,'+','2=1D3=2I4='),1)
        self.assertEqual(a,b)
    def test_swapped_reverse_indels(self):
        a=normalize(paf('a',10,20,'b',30,41,'-','4=2D3=1I2='),0)
        b=normalize(paf('b',30,41,'a',10,20,'-','2=1D3=2I4='),1)
        self.assertEqual(a,b)
        self.assertEqual(a[-1],(('=',2),('I',1),('=',3),('D',2),('=',4)))
    def test_equivalent_run_segmentation(self):
        a=normalize(paf('a',0,100,'b',0,100,'+','30=70='),0)
        b=normalize(paf('a',0,100,'b',0,100,'+','100='),0)
        self.assertEqual(a,b)
    def test_intervals(self):
        self.assertEqual(merge([(10,20),(5,12),(20,25),(30,35)]),[(5,25),(30,35)])
        self.assertEqual(intersect([(5,25),(30,35)],[(0,10),(15,33)]),18)
    def test_repeat_counts_and_subpath_deduplication(self):
        with TemporaryDirectory() as tmp:
            path=Path(tmp)/'x.paf'
            lines=[paf('a',0,200,'b',0,200,'+','200='),
                   paf('a',0,200,'b',0,200,'+','200='),
                   paf('a',100,200,'b',100,200,'+','100='),
                   paf('a',0,200,'b',300,500,'+','200=')]
            path.write_text('\n'.join('\t'.join(x) for x in lines)+'\n')
            p=Placements();p.add_file(path,0);s=p.finalize()
            self.assertEqual(s['raw_records'],4);self.assertEqual(s['unique_paths'],3)
            self.assertEqual(s['aligned_pair_positions'],400)
            self.assertEqual(s['covered_a_bases'],200)
            self.assertEqual(s['mean_placements_per_paired_a_base'],2)
            self.assertEqual(s['max_placements_per_a_base'],2)
            self.assertEqual(s['multi_placement_a_bases'],200)
    def test_reverse_diagonal_pairs(self):
        with TemporaryDirectory() as tmp:
            path=Path(tmp)/'x.paf';path.write_text('\t'.join(paf('a',0,100,'b',100,200,'-','100='))+'\n')
            p=Placements();p.add_file(path,0);s=p.finalize()
            self.assertEqual(p.pairs,{('a','b',True,-200):[(0,100)]})
            self.assertEqual(s['aligned_pair_positions'],100)

if __name__=='__main__':unittest.main()
