#!/usr/bin/env python3
"""Cross-compile a self-contained scalar/AVX2 validation bundle for Linux."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out',type=Path,required=True,help='fresh bundle directory outside the repository')
    args=parser.parse_args();here=Path(__file__).resolve().parent;repo=here.parents[2];out=args.out.resolve()
    archive=Path(str(out)+'.tar.gz')
    if out.is_relative_to(repo):parser.error('--out must be outside the repository')
    if out.exists() or archive.exists():parser.error('choose a fresh directory and archive name')
    out.mkdir(parents=True)
    env=os.environ.copy()
    env.update(CGO_ENABLED='0',GOOS='linux',GOARCH='amd64',GOAMD64='v1')
    commands=[]
    for label,experiment in [('scalar',''),('simd','simd')]:
        env['GOEXPERIMENT']=experiment
        for package,prefix in [('./align','align'),('./benchmarks/alignment','alignment')]:
            command=['go','test','-trimpath','-ldflags=-s -w','-c','-o',str(out/f'{prefix}-{label}.test'),package]
            subprocess.run(command,cwd=repo,env=env,check=True);commands.append({'command':command,'GOEXPERIMENT':experiment})
    env['GOEXPERIMENT']=''
    command=['go','build','-trimpath','-ldflags=-s -w','-o',str(out/'runner'),'./benchmarks/alignment/remote']
    subprocess.run(command,cwd=repo,env=env,check=True);commands.append({'command':command,'GOEXPERIMENT':''})
    records=json.loads((repo/'benchmarks/alignment/results.json').read_text())
    source_files=[repo/'go.mod',repo/'go.sum',*sorted(repo.rglob('*.go'))]
    hashes={str(p.relative_to(repo)):hashlib.sha256(p.read_bytes()).hexdigest() for p in source_files if '.git' not in p.parts and '.cache' not in p.parts}
    source_hash=hashlib.sha256(json.dumps(hashes,sort_keys=True).encode()).hexdigest()
    datasets=records['references']
    (out/'reference.json').write_text(json.dumps({'Source':'worktree-sha256:'+source_hash,'Datasets':datasets},indent=2)+'\n')
    (out/'build.json').write_text(json.dumps({'go':subprocess.check_output(['go','version'],text=True).strip(),'environment':{k:env[k] for k in ['CGO_ENABLED','GOOS','GOARCH','GOAMD64']},'commands':commands,'source_sha256':hashes},indent=2)+'\n')
    shutil.copy2(here/'BUNDLE_README.md',out/'README.md')
    (out/'run.sh').write_text('#!/bin/sh\nset -eu\ntask_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\nexec "$task_dir/runner" "$@"\n')
    (out/'run.sh').chmod(0o755)
    checksum=''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(out.iterdir()) if p.is_file())
    (out/'SHA256SUMS').write_text(checksum)
    with tarfile.open(archive,'w:gz') as tar:tar.add(out,arcname=out.name)
    print(archive)

if __name__=='__main__':main()
