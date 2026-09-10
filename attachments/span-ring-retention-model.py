"""Source-level single-owner count model; no Go execution or host leak claim."""
from dataclasses import dataclass
import json
from pathlib import Path
@dataclass
class Ring:
 cap:int
 n:int=0
 dead:bool=False
class Queue:
 def __init__(self,minimum=1024,maximum=131072):
  self.local=0;self.puts=0;self.chain=[];self.all=[];self.minimum=minimum;self.maximum=maximum;self.phase='off';self.created=0
 def new(self,cap):
  assert self.phase=='mark';r=Ring(cap);self.all.append(r);self.created+=1;return r
 def drain(self,n):
  assert 0<n<=self.local
  self.puts=0
  if not self.chain:self.chain.append(self.new(self.minimum))
  r=self.chain[-1]
  if r.n+n>r.cap:r=self.new(min(2*r.cap,self.maximum));self.chain.append(r)
  assert r.n+n<=r.cap
  r.n+=n;self.local-=n
 def put(self):
  if self.local<256:
   self.local+=1;self.puts+=1
   if self.puts>=64:
    self.puts=0;n=min(self.local//2,16)
    if n>4 and all(r.n==0 for r in self.chain):self.drain(n)
  else:self.drain(128);self.local+=1
 def get(self):
  if self.local:self.local-=1;return True
  while self.chain:
   r=self.chain[0]
   if r.n:
    n=min(r.n-r.n//2,128);r.n-=n;self.local=n-1;return True
   if len(self.chain)==1:return False
   self.chain.pop(0);r.dead=True
  return False
 def cleanup(self):
  if self.phase!='off':return
  self.all=[r for r in self.all if not r.dead]
 def cycle(self,items,cleanup=False):
  self.phase='mark'
  for _ in range(items):self.put()
  peak_live=len(self.chain)
  consumed=0
  while self.get():consumed+=1
  assert consumed==items and self.local==0 and len(self.chain)==1 and self.chain[0].n==0
  self.phase='off'
  if cleanup:self.cleanup()
  return {'items':items,'peak_active_chain':peak_live,'active_after':len(self.chain),'dead_retained':sum(r.dead for r in self.all),'mapped_pointer_bytes':sum(8*r.cap for r in self.all),'created_total':self.created}
# Use exact source capacities and spills. Fixed peak work for every modeled cycle.
items=3*131072+256
q=Queue();without=[q.cycle(items) for _ in range(12)]
c=Queue();with_cleanup=[c.cycle(items,True) for _ in range(12)]
assert all(x['active_after']==1 for x in without)
assert all(without[i]['dead_retained']>without[i-1]['dead_retained'] for i in range(1,len(without)))
assert all(x['dead_retained']==0 for x in with_cleanup)
assert len({x['mapped_pointer_bytes'] for x in with_cleanup})==1
# A cleanup call during mark refuses, exactly as the source phase guard does.
q.phase='mark';before=len(q.all);q.cleanup();assert len(q.all)==before
out={'profile':'Go1.26.7 GreenTea spanQueue single-producer/single-consumer count abstraction','constant_peak_queued_items_per_cycle':items,'without_cleanup':without,'with_off_phase_cleanup':with_cleanup,'mark_phase_cleanup_refuses':True,'conclusion':'A bound on peak active queue depth plus completed mark/sweep does not alone bound dead ring history. The distinct cleanup path must be reached or replacement frequency separately bounded.','limits':'This models queue operations and the phase guard, not scheduler, complete collector, reachable evaluator object graph, elapsed time or actual heap behavior. It is not a native leak witness or proof that an admitted execution realizes this trace.'}
Path('/tmp/tail4043-span-ring-retention-model-f7.json').write_text(json.dumps(out,indent=2));print('PASS exact-capacity12cycles; dead',without[0]['dead_retained'],'->',without[-1]['dead_retained'],';off-phase cleanup keeps',with_cleanup[-1]['mapped_pointer_bytes'],'ring bytes')
