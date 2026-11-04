import os
import json
from pathlib import Path
import sys
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch
import subprocess
from run import execute, faqt_settings,file_sha256, main as run_main

@unittest.skipUnless(hasattr(os,'wait4'),'requires POSIX wait4')
class TestProcessAccounting(unittest.TestCase):
    def test_exit_status_and_separate_logs(self):
        with TemporaryDirectory() as tmp:
            stem=Path(tmp)/'child'
            r=execute([sys.executable,'-c','import sys;print("out");print("err",file=sys.stderr);sys.exit(7)'],os.environ.copy(),stem,5)
            self.assertEqual(r['exit_code'],7);self.assertFalse(r['timeout'])
            self.assertEqual(Path(str(stem)+'.stdout').read_text(),'out\n')
            self.assertEqual(Path(str(stem)+'.stderr').read_text(),'err\n')
            self.assertGreater(r['wall_seconds'],0)
            self.assertGreaterEqual(r['user_seconds'],0)
    def test_timeout(self):
        with TemporaryDirectory() as tmp:
            r=execute([sys.executable,'-c','import time;time.sleep(2)'],os.environ.copy(),Path(tmp)/'child',.05)
            self.assertTrue(r['timeout']);self.assertNotEqual(r['exit_code'],0)
            self.assertLess(r['wall_seconds'],1.5)
    def test_interrupt_reaps_child(self):
        children=[];original=subprocess.Popen
        def capture(*args,**kwargs):
            child=original(*args,**kwargs);children.append(child);return child
        with TemporaryDirectory() as tmp,patch('run.subprocess.Popen',side_effect=capture),patch('run.time.sleep',side_effect=KeyboardInterrupt):
            with self.assertRaises(KeyboardInterrupt):
                execute([sys.executable,'-c','import time;time.sleep(2)'],os.environ.copy(),Path(tmp)/'child',5)
        self.assertIsNotNone(children[0].returncode)
        with self.assertRaises(ChildProcessError):os.waitpid(children[0].pid,os.WNOHANG)
    def test_streamed_hash(self):
        with TemporaryDirectory() as tmp:
            p=Path(tmp)/'input';p.write_bytes(b'abc')
            self.assertEqual(file_sha256(p),'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')

class TestFaqtProfiles(unittest.TestCase):
    def test_explicit_policy_and_matching_budget(self):
        for profile,budget,mode in [('faqt-full',123456,'dp'),('faqt-adaptive',123456,'adaptive'),('faqt-adaptive-near',123456,'adaptive')]:
            with self.subTest(profile=profile):
                settings=faqt_settings(profile,123456)
                self.assertEqual(settings['FAQT_MAX_ANCHORS'],str(budget))
                self.assertEqual(settings['FAQT_BENCH_END_EXTENSION'],mode)
                self.assertEqual(settings['FAQT_BENCH_REDUNDANCY'],'near' if profile=='faqt-adaptive-near' else 'exact')

@unittest.skipUnless(hasattr(os,'wait4'),'requires POSIX wait4')
class TestAdaptiveBenchmarkCLI(unittest.TestCase):
    def test_go_only_profiles_use_one_process_and_override_inherited_policy(self):
        with TemporaryDirectory() as tmp:
            root=Path(tmp)
            for name in ['31585_6#49.fasta','barcode10_ICESp23_spliced.fasta']:
                (root/name).write_text('>x\nACGT\n')
            binary=root/'fake.test'
            binary.write_text('#!/usr/bin/env python3\nimport os,json\nstem=os.environ["FAQT_BENCH_OUTPUT"]\nopen(stem+".paf","w").close()\njson.dump({"Policy":os.environ["FAQT_BENCH_END_EXTENSION"],"Budget":os.environ["FAQT_MAX_ANCHORS"],"Redundancy":os.environ["FAQT_BENCH_REDUNDANCY"]},open(stem+".json","w"))\n')
            binary.chmod(0o755)
            out=root/'results'
            argv=['run.py','--debug-dir',str(root),'--out',str(out),'--dataset','debug','--faqt-bin',str(binary),'--minimap2','','--profile','faqt-full','--profile','faqt-adaptive','--profile','faqt-adaptive-near','--raised-anchors','123456','--runs','1']
            with patch.object(sys,'argv',argv),patch('run.platform.platform',return_value='test-os'),patch('run.platform.processor',return_value='test-cpu'),patch('run.subprocess.check_output',return_value='test-tool\n'),patch.dict(os.environ,{'FAQT_BENCH_END_EXTENSION':'snp1','FAQT_BENCH_REDUNDANCY':'near'}):
                run_main()
            rows=json.loads((out/'timings.json').read_text())
            self.assertEqual(len(rows),3)
            for row,policy in zip(rows,['dp','adaptive','adaptive']):
                self.assertEqual(len(row['legs']),1)
                self.assertEqual(row['legs'][0]['exit_code'],0)
                self.assertEqual(row['legs'][0]['faqt'],{'Policy':policy,'Budget':'123456','Redundancy':'near' if row['profile']=='faqt-adaptive-near' else 'exact'})
            self.assertIsNone(json.loads((out/'metadata.json').read_text())['minimap2'])

if __name__=='__main__':unittest.main()
