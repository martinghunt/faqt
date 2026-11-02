#!/usr/bin/env python3
"""Serial, process-isolated alignment benchmarks. Python 3, macOS/Linux."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import sys
import time

DATASETS={
 'debug':('31585_6#49.fasta','barcode10_ICESp23_spliced.fasta'),
 'shigella':('repeats.CP026802.1.fa','repeats.CP151312.1.fa'),
}
PROFILES=['faqt-full','faqt-adaptive','mm-similar-both']

def faqt_settings(profile,raised_anchors):
    return {'FAQT_MAX_ANCHORS':str(raised_anchors),
            'FAQT_BENCH_END_EXTENSION':'adaptive' if profile=='faqt-adaptive' else 'dp'}

def file_sha256(path):
    digest=hashlib.sha256()
    with open(path,'rb') as handle:
        for block in iter(lambda:handle.read(1024*1024),b''):digest.update(block)
    return digest.hexdigest()

def execute(command,env,stem,timeout):
    start=time.perf_counter()
    with open(str(stem)+'.stdout','wb') as out,open(str(stem)+'.stderr','wb') as err:
        child=subprocess.Popen(command,env=env,stdout=out,stderr=err,start_new_session=True)
        timed_out=False
        try:
            while True:
                pid,status,usage=os.wait4(child.pid,os.WNOHANG)
                if pid:break
                if time.perf_counter()-start>timeout:
                    timed_out=True
                    os.killpg(child.pid,signal.SIGTERM)
                    pid,status,usage=os.wait4(child.pid,0)
                    break
                time.sleep(.01)
        except BaseException:
            # Popen starts a new session so Ctrl-C on the parent must explicitly
            # terminate and reap its benchmark child as well.
            try:os.killpg(child.pid,signal.SIGTERM)
            except ProcessLookupError:pass
            try:
                _,status,_=os.wait4(child.pid,0)
                child.returncode=os.waitstatus_to_exitcode(status)
            except ChildProcessError:pass
            raise
        child.returncode=os.waitstatus_to_exitcode(status)
    rss=usage.ru_maxrss if sys.platform=='darwin' else usage.ru_maxrss*1024
    return {'command':command,'wall_seconds':time.perf_counter()-start,
      'user_seconds':usage.ru_utime,'system_seconds':usage.ru_stime,
      'peak_rss_bytes':rss,'exit_code':child.returncode,'timeout':timed_out}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--debug-dir',type=Path,required=True)
    p.add_argument('--out',type=Path,required=True)
    p.add_argument('--faqt-bin',type=Path)
    p.add_argument('--minimap2',default=shutil.which('minimap2'))
    p.add_argument('--repo',type=Path,default=Path(__file__).resolve().parents[2])
    p.add_argument('--dataset',action='append',choices=DATASETS)
    p.add_argument('--profile',action='append',choices=PROFILES)
    p.add_argument('--runs',type=int,default=3)
    p.add_argument('--timeout',type=float,default=300)
    p.add_argument('--raised-anchors',type=int,default=20000000)
    args=p.parse_args()
    if args.runs<1 or args.timeout<=0:p.error('runs and timeout must be positive')
    if not hasattr(os,'wait4'):p.error('process resource accounting requires macOS or Linux')
    profiles=args.profile or PROFILES
    needs_minimap2=any(profile.startswith('mm-') for profile in profiles)
    if needs_minimap2 and not args.minimap2:p.error('minimap2 executable is required for minimap2 profiles')
    args.out.mkdir(parents=True,exist_ok=True)
    if args.faqt_bin is None:
        args.faqt_bin=(args.out/'alignment.test').resolve()
        env=os.environ.copy();env['CGO_ENABLED']='0'
        subprocess.run(['go','test','-c','-o',str(args.faqt_bin),'./benchmarks/alignment'],cwd=args.repo,env=env,check=True)
    args.faqt_bin=args.faqt_bin.resolve()
    metadata={'platform':platform.platform(),'processor':platform.processor(),
       'minimap2':subprocess.check_output([args.minimap2,'--version'],text=True).strip() if needs_minimap2 else None,
       'go':subprocess.check_output(['go','version'],text=True).strip(),
       'faqt_revision':subprocess.check_output(['git','rev-parse','HEAD'],cwd=args.repo,text=True).strip(),
       'runs':args.runs,'timeout_seconds':args.timeout,'raised_anchors':args.raised_anchors,
       'inputs':{}}
    for ds in args.dataset or DATASETS:
        metadata['inputs'][ds]=[]
        for name in DATASETS[ds]:
            path=(args.debug_dir/name).resolve()
            metadata['inputs'][ds].append({'path':str(path),'sha256':file_sha256(path)})
    (args.out/'metadata.json').write_text(json.dumps(metadata,indent=2)+'\n')
    results=[]
    for ds in args.dataset or DATASETS:
        a,b=[(args.debug_dir/name).resolve() for name in DATASETS[ds]]
        for profile in profiles:
            for repeat in range(args.runs):
                case={'dataset':ds,'profile':profile,'repeat':repeat+1,'legs':[]}
                directions=[0] if profile.startswith('faqt') else [0,1]
                for direction in directions:
                    name=f'{ds}.{profile}.{repeat+1}.{direction}'
                    stem=(args.out/name).resolve()
                    env=os.environ.copy();env['GOMAXPROCS']='1'
                    if profile.startswith('faqt'):
                        env.update(FAQT_BENCH_A=str(a),FAQT_BENCH_B=str(b),FAQT_BENCH_OUTPUT=str(stem))
                        env.update(faqt_settings(profile,args.raised_anchors))
                        cmd=[str(args.faqt_bin),'-test.run=^TestComparison$','-test.v','-test.timeout=11m']
                    else:
                        cmd=[args.minimap2,'-x','asm5','-c','--eqx','-P','-t','1']
                        cmd+=['-k','15','-w','10','-A','2','-B','4','-O','4,24','-E','2,1',
                              '-g','2000','-r','2000,2000','-z','200,200','-m','40','-s','40','-n','1',
                              '-f','100000000','-U','100000000,100000000','--q-occ-frac','0','--no-long-join','--rmq=no']
                        cmd += [str(a if direction else b),str(b if direction else a)]
                    print(f'RUN {name}',flush=True)
                    result=execute(cmd,env,stem,args.timeout)
                    result.update(stem=name,direction=direction)
                    if not profile.startswith('faqt'):
                        Path(str(stem)+'.stdout').replace(str(stem)+'.paf')
                    else:
                        summary_path=Path(str(stem)+'.json')
                        if summary_path.exists():result['faqt']=json.loads(summary_path.read_text())
                    case['legs'].append(result)
                    print(f'  {result["wall_seconds"]:.3f}s, RSS {result["peak_rss_bytes"]/2**20:.1f} MiB, exit {result["exit_code"]}',flush=True)
                    if result['exit_code']!=0:break
                results.append(case)
                (args.out/'timings.json').write_text(json.dumps(results,indent=2)+'\n')
                if any(x['exit_code']!=0 for x in case['legs']):
                    print('  Failed/incomplete; no repeated samples for this profile.',flush=True)
                    break
    print('Timing measurements saved to',args.out,flush=True)

if __name__=='__main__':main()
