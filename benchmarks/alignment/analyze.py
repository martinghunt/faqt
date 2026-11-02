#!/usr/bin/env python3
"""Normalize PAF axes and compare coverage of strand-specific aligned base pairs."""
import argparse
from collections import defaultdict
import json
from pathlib import Path
import re
import statistics
import sys

CIGAR=re.compile(r'(\d+)([=XID])')

def cigar_ops(cigar):
    ops=[(code,int(n)) for n,code in CIGAR.findall(cigar)]
    if not ops or ''.join(f'{n}{c}' for c,n in ops)!=cigar or any(n<1 for _,n in ops):
        raise ValueError('expected an extended assembly CIGAR: '+cigar)
    return ops

def normalize(fields,direction):
    q,qs,qe=sys.intern(fields[0]),int(fields[2]),int(fields[3])
    t,ts,te=sys.intern(fields[5]),int(fields[7]),int(fields[8])
    if fields[4] not in ('+','-') or not (0<=qs<=qe<=int(fields[1]) and 0<=ts<=te<=int(fields[6])):raise ValueError('invalid PAF intervals or strand')
    reverse=fields[4]=='-'
    cg=next((s[5:] for s in fields[12:] if s.startswith('cg:Z:')),None)
    if cg is None:raise ValueError('missing CIGAR')
    ops=cigar_ops(cg)
    # PAF CIGAR follows target-forward order. Our normalized operations follow
    # genome A forwards and genome B forwards/backwards according to strand.
    if reverse:ops.reverse()
    if direction:
        q,t,qs,qe,ts,te=t,q,ts,te,qs,qe
        if reverse:ops.reverse()
        ops=[({'I':'D','D':'I'}.get(c,c),n) for c,n in ops]
    compact=[]
    for code,n in ops:
        if compact and compact[-1][0]==code:compact[-1]=(code,compact[-1][1]+n)
        else:compact.append((code,n))
    ops=compact
    if sum(n for c,n in ops if c!='D')!=qe-qs or sum(n for c,n in ops if c!='I')!=te-ts:
        raise ValueError('CIGAR does not consume PAF intervals')
    return q,qs,qe,t,ts,te,reverse,tuple(ops)

def merge(intervals):
    out=[]
    for start,end in sorted(intervals):
        if out and start<=out[-1][1]:out[-1]=(out[-1][0],max(out[-1][1],end))
        else:out.append((start,end))
    return out

def intersect(a,b):
    i=j=total=0
    while i<len(a) and j<len(b):
        total+=max(0,min(a[i][1],b[j][1])-max(a[i][0],b[j][0]))
        if a[i][1]<b[j][1]:i+=1
        else:j+=1
    return total

def finish(intervals):return {key:merge(value) for key,value in intervals.items()}
def length(intervals):return sum(end-start for spans in intervals.values() for start,end in spans)

class Placements:
    def __init__(self):
        self.raw=0;self.qualifying=0;self.paths=set();self.endpoints=set()
        self.cover_a=defaultdict(list);self.cover_b=defaultdict(list);self.pairs=defaultdict(list)
    def add_file(self,path,direction):
        with open(path) as handle:
            for line in handle:
                if not line.strip():continue
                self.raw+=1
                a,as_,ae,b,bs,be,rev,ops=normalize(line.rstrip('\n').split('\t'),direction)
                columns=sum(n for _,n in ops);matches=sum(n for c,n in ops if c=='=')
                score=sum(2*n if c=='=' else -4*n if c=='X' else -min(4+2*n,24+n) for c,n in ops)
                if min(ae-as_,be-bs)<100 or matches/columns<.7 or score<40:continue
                self.qualifying+=1
                endpoint=(a,as_,ae,b,bs,be,rev)
                key=endpoint+(ops,)
                if key in self.paths:continue
                self.paths.add(key);self.endpoints.add(endpoint)
                self.cover_a[a].append((as_,ae));self.cover_b[b].append((bs,be))
                q,t=as_,-be if rev else bs
                for c,n in ops:
                    if c in '=X':
                        self.pairs[(a,b,rev,t-q)].append((q,q+n));q+=n;t+=n
                    elif c=='I':q+=n
                    else:t+=n
    def finalize(self):
        self.cover_a=finish(self.cover_a);self.cover_b=finish(self.cover_b);self.pairs=finish(self.pairs)
        paired_a=defaultdict(list);events=defaultdict(list)
        for key,spans in self.pairs.items():
            for start,end in spans:
                paired_a[key[0]].append((start,end));events[key[0]].extend(((start,1),(end,-1)))
        paired_bases=length(finish(paired_a));pair_count=length(self.pairs)
        maximum=multi=0
        for contig_events in events.values():
            changes=defaultdict(int)
            for pos,change in contig_events:changes[pos]+=change
            current=last=0
            for pos,change in sorted(changes.items()):
                if current>=2:multi+=pos-last
                current+=change;maximum=max(maximum,current);last=pos
        return {'raw_records':self.raw,'qualifying_records':self.qualifying,
          'unique_paths':len(self.paths),'unique_endpoints':len(self.endpoints),
          'covered_a_bases':length(self.cover_a),'covered_b_bases':length(self.cover_b),
          'paired_a_bases':paired_bases,'aligned_pair_positions':pair_count,
          'mean_placements_per_paired_a_base':pair_count/paired_bases if paired_bases else 0,
          'max_placements_per_a_base':maximum,'multi_placement_a_bases':multi}

def analyze(out):
    timings=json.loads((out/'timings.json').read_text());result=[];objects={}
    for ds,profile in sorted({(r['dataset'],r['profile']) for r in timings}):
        rows=[r for r in timings if (r['dataset'],r['profile'])==(ds,profile)]
        good=[r for r in rows if all(l['exit_code']==0 for l in r['legs'])]
        entry={'dataset':ds,'profile':profile,'completed_runs':len(good),'attempted_runs':len(rows)}
        if not good:
            errors=[]
            for row in rows:
                for leg in row['legs']:
                    message=leg.get('faqt',{}).get('Error') or (out/(leg['stem']+'.stderr')).read_text()[-2000:]
                    if leg.get('timeout'):message=f'Timed out after {leg["wall_seconds"]:.1f} seconds'
                    errors.append(message or f'Process exited with code {leg["exit_code"]}')
            entry['errors']=errors;entry['failure_wall_seconds']=sum(l['wall_seconds'] for l in rows[0]['legs']);entry['peak_rss_mib']=max(l['peak_rss_bytes'] for r in rows for l in r['legs'])/2**20;result.append(entry);continue
        walls=[sum(l['wall_seconds'] for l in r['legs']) for r in good]
        entry.update(wall_median=statistics.median(walls),wall_min=min(walls),wall_max=max(walls),
          cpu_median=statistics.median(sum(l['user_seconds']+l['system_seconds'] for l in r['legs']) for r in good),
          peak_rss_mib=max(l['peak_rss_bytes'] for r in good for l in r['legs'])/2**20)
        if profile.startswith('faqt'):
            entry['prepare_median']=statistics.median(r['legs'][0]['faqt']['PrepareSeconds'] for r in good)
            entry['align_median']=statistics.median(r['legs'][0]['faqt']['AlignSeconds'] for r in good)
            entry['allocated_mib_median']=statistics.median(r['legs'][0]['faqt']['AllocatedBytes'] for r in good)/2**20
        p=Placements()
        for leg in good[0]['legs']:p.add_file(out/(leg['stem']+'.paf'),leg['direction'])
        entry.update(p.finalize());objects[ds,profile]=p;result.append(entry)
    for entry in result:
        if not entry['completed_runs']:continue
        candidate=objects[entry['dataset'],entry['profile']]
        entry['pairwise_overlap']={}
        for (ds,profile),reference in objects.items():
            if ds!=entry['dataset'] or profile==entry['profile']:continue
            common=sum(intersect(spans,reference.pairs.get(key,[])) for key,spans in candidate.pairs.items())
            denom=length(reference.pairs)
            entry['pairwise_overlap'][profile]={'shared_pair_positions':common,
                'fraction_of_other_pairs':common/denom if denom else 0,
                'fraction_of_own_pairs':common/length(candidate.pairs) if candidate.pairs else 0}
    (out/'summary.json').write_text(json.dumps(result,indent=2)+'\n')
    return result

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('out',type=Path);args=p.parse_args()
    for r in analyze(args.out):
        if not r['completed_runs']:print(r['dataset'],r['profile'],'FAILED',r['errors'][0].splitlines()[0]);continue
        print(f'{r["dataset"]:9s} {r["profile"]:18s} {r["wall_median"]:8.3f}s {r["peak_rss_mib"]:8.1f}MiB paths={r["unique_paths"]:7d} A={r["covered_a_bases"]:8d} B={r["covered_b_bases"]:8d} pairs={r["aligned_pair_positions"]:10d} mean={r["mean_placements_per_paired_a_base"]:.3f}')

if __name__=='__main__':main()
