#!/usr/bin/env python3
"""Serial full-genome kernel comparisons, using a binary from build.py."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--study',type=Path,required=True)
 p.add_argument('--debug-dir',type=Path,required=True)
 p.add_argument('--dataset',choices=['debug','repeats'],required=True)
 p.add_argument('--kernel',action='append',choices=['scalar','fast','simd','wide','wfa','hybrid','wfa2'])
 p.add_argument('--runs',type=int,default=1)
 p.add_argument('--capture',action='store_true')
 args=p.parse_args()
 if args.runs<1:p.error('--runs must be positive')
 if not hasattr(os,'wait4'):p.error('resource accounting requires POSIX wait4')
 study=args.study.resolve();out=study/'results';out.mkdir(exist_ok=True)
 a,b=('31585_6#49.fasta','barcode10_ICESp23_spliced.fasta') if args.dataset=='debug' else ('repeats.CP026802.1.fa','repeats.CP151312.1.fa')
 for mode in args.kernel or ['scalar','fast','simd','wfa','hybrid']:
  for run in range(1,args.runs+1):
   stem=out/f'{args.dataset}.{mode}.{run}'
   env=os.environ.copy();env.update(GOMAXPROCS='1',FAQT_KERNEL=mode,FAQT_BENCH_A=str((args.debug_dir/a).resolve()),FAQT_BENCH_B=str((args.debug_dir/b).resolve()),FAQT_MAX_ANCHORS='20000000',FAQT_BENCH_OUTPUT=str(stem),FAQT_BENCH_TIMEOUT='10m')
   env.pop('FAQT_CAPTURE',None);env.pop('FAQT_BENCH_END_EXTENSION',None)
   if args.capture and mode=='scalar' and run==1:env['FAQT_CAPTURE']=str(out/f'{args.dataset}.samples.jsonl')
   print('START',args.dataset,mode,run,flush=True);start=time.perf_counter()
   with Path(str(stem)+'.log').open('w') as log:
    child=subprocess.Popen([str(study/'assembly.test'),'-test.run=^TestComparison$','-test.v'],env=env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
    try:_,status,usage=os.wait4(child.pid,0)
    except BaseException:
     import signal
     os.killpg(child.pid,signal.SIGTERM);_,status,_=os.wait4(child.pid,0);child.returncode=os.waitstatus_to_exitcode(status);raise
    child.returncode=os.waitstatus_to_exitcode(status)
   result={'dataset':args.dataset,'kernel':mode,'run':run,'wall_seconds':time.perf_counter()-start,'cpu_seconds':usage.ru_utime+usage.ru_stime,'peak_rss_bytes':usage.ru_maxrss*(1 if os.uname().sysname=='Darwin' else 1024),'exit_code':child.returncode}
   summary=Path(str(stem)+'.json');paf=Path(str(stem)+'.paf')
   if summary.exists():result.update(json.loads(summary.read_text()))
   if paf.exists():result['paf_sha256']=hashlib.sha256(paf.read_bytes()).hexdigest()
   Path(str(stem)+'.timing.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result),flush=True)

if __name__=='__main__':main()
