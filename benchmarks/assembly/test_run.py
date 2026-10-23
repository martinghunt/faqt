import os
from pathlib import Path
import sys
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch
import subprocess
from run import execute,file_sha256

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

if __name__=='__main__':unittest.main()
