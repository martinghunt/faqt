#!/usr/bin/env python3
"""Replay faqt benchmark PAFs against the original DNA and check repeatability."""
import argparse
import json
from pathlib import Path
from analyze import cigar_ops
from run import file_sha256

COMPLEMENT=str.maketrans('ACGTN','TGCAN')

def read_fasta(path):
    records={};name=None;parts=[]
    def finish():
        if name is None:return
        if name in records:raise ValueError('PAF replay requires unique contig names')
        raw=''.join(parts).upper().replace('U','T')
        records[name]=''.join(c if c in 'ACGT' else 'N' for c in raw)
    with open(path) as handle:
        for line in handle:
            if line.startswith('>'):
                finish();name=line[1:].split()[0];parts=[]
            else:parts.append(line.strip())
    finish();return records

def replay(path,a,b):
    reverse={};count=0
    with open(path) as handle:
        for line in handle:
            r=line.rstrip('\n').split('\t');q,t=a[r[0]],b[r[5]]
            qs,qe,ts,te=map(int,(r[2],r[3],r[7],r[8]))
            if int(r[1])!=len(q) or int(r[6])!=len(t):raise ValueError('sequence length changed')
            if r[4]=='-':
                if r[0] not in reverse:reverse[r[0]]=q.translate(COMPLEMENT)[::-1]
                q=reverse[r[0]];qs,qe=len(q)-qe,len(q)-qs
            i,j=qs,ts;matches=columns=0
            cg=next(v[5:] for v in r[12:] if v.startswith('cg:Z:'))
            for code,n in cigar_ops(cg):
                columns+=n
                if code in '=X':
                    x,y=q[i:i+n],t[j:j+n]
                    if len(x)!=n or len(y)!=n:raise ValueError('paired operation past sequence')
                    if code=='=':
                        if x!=y or 'N' in x:raise ValueError('incorrect exact match')
                        matches+=n
                    elif any(c==d and c!='N' for c,d in zip(x,y)):raise ValueError('incorrect substitution')
                    i+=n;j+=n
                elif code=='I':i+=n
                else:j+=n
            if (i,j,matches,columns)!=(qe,te,int(r[9]),int(r[10])):raise ValueError('inconsistent coordinates or identity')
            count+=1
    return count

def verify(out):
    metadata=json.loads((out/'metadata.json').read_text());timings=json.loads((out/'timings.json').read_text());result=[]
    for ds,profile in sorted({(r['dataset'],r['profile']) for r in timings if r['profile'].startswith('faqt')}):
        good=[r for r in timings if (r['dataset'],r['profile'])==(ds,profile) and all(l['exit_code']==0 for l in r['legs'])]
        if not good:continue
        inputs=metadata['inputs'][ds]
        for item in inputs:
            if file_sha256(item['path'])!=item['sha256']:raise ValueError('input changed since benchmarking')
        a,b=[read_fasta(i['path']) for i in inputs]
        pafs=[out/(r['legs'][0]['stem']+'.paf') for r in good]
        hashes=[file_sha256(p) for p in pafs]
        if len(set(hashes))!=1:raise ValueError('identical faqt runs produced different PAFs')
        count=replay(pafs[0],a,b)
        result.append({'dataset':ds,'profile':profile,'verified_paths':count,'identical_runs':len(good),'paf_sha256':hashes[0]})
    (out/'verification.json').write_text(json.dumps(result,indent=2)+'\n');return result

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('out',type=Path);args=p.parse_args()
    for r in verify(args.out):print(r)
if __name__=='__main__':main()
