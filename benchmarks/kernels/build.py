#!/usr/bin/env python3
"""Build optional scalar/SIMD/WFA experiments in an isolated source copy."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--out',type=Path,required=True)
 p.add_argument('--scan-containment',action='store_true',help='use the pre-index containment scans for an ablation benchmark')
 p.add_argument('--wfa2',type=Path,help='WFA2 source tree with lib/libwfa.a built')
 args=p.parse_args();here=Path(__file__).resolve().parent;repo=here.parents[1]
 out=args.out.resolve()
 if out.is_relative_to(repo):p.error("--out must be outside the repository")
 out.mkdir(parents=True,exist_ok=True);source=out/'source'
 if source.exists():p.error('output source directory already exists; choose a fresh --out')
 if args.wfa2 and not (args.wfa2/'lib/libwfa.a').exists():p.error('build WFA2 lib/libwfa.a first')
 shutil.copytree(repo,source,ignore=shutil.ignore_patterns('.git','.cache','build','dist','__pycache__','*.test'))
 dp=source/'align/dp.go';text=dp.read_text();marker='\nfunc alignDP('
 if marker not in text:p.error('expected the platform dispatch wrapper in align/dp.go')
 dp.write_text(text[:text.index(marker)]+'\n')
 for asset in (here/'assets').glob('*.go.txt'):
  text=asset.read_text().replace('@WFA2_ROOT@',str(args.wfa2.resolve()) if args.wfa2 else '/unused-wfa2')
  (source/'align'/asset.name.removesuffix('.txt')).write_text(text)
 if args.scan_containment:
  for name in ['chain','refine']:(source/'align'/(name+'.go')).write_bytes((here/'assets'/(name+'.scan.txt')).read_bytes())
 env=os.environ.copy();env['GOEXPERIMENT']='simd'
 flags=['-tags','studywfa2'] if args.wfa2 else []
 for package,name in [('./align','align.test'),('./benchmarks/assembly','assembly.test')]:
  subprocess.run(['go','test',*flags,'-c','-o',str(out/name),package],cwd=source,env=env,check=True)
 print(out)

if __name__=='__main__':main()
