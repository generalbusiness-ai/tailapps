import json,pathlib,struct,hashlib,subprocess,zlib,os
root=pathlib.Path(os.environ.get('TAIL3925_COMPARE_ROOT') or json.load(open('/tmp/tail3925-build-session.json'))['root']);files=[root/'tailapp-baseline',root/'tailapp-1'];data=[p.read_bytes() for p in files]
ids=[subprocess.check_output(['go','tool','buildid',str(p)],text=True).strip().encode() for p in files]
def sections(b):
 p=32;out={}
 for _ in range(struct.unpack_from('<I',b,16)[0]):
  cmd,n=struct.unpack_from('<II',b,p)
  if cmd==0x19:
   for j in range(struct.unpack_from('<I',b,p+64)[0]):
    v=struct.unpack_from('<16s16sQQIIIIIIII',b,p+72+j*80);name=v[1].split(b'\0')[0].decode()+','+v[0].split(b'\0')[0].decode();out[name]=v
  p+=n
 return out
ss=[sections(b) for b in data];rows=[]
for name in sorted(ss[0]):
 a,b=[x[name] for x in ss];x=data[0][a[4]:a[4]+a[3]];y=data[1][b[4]:b[4]+b[3]]
 if a[8]&255 in [1,12,18]:rows.append({'section':name,'zero_fill_equal':a==b});continue
 if x.startswith(b'ZLIB'):x=zlib.decompress(x[12:])
 if y.startswith(b'ZLIB'):y=zlib.decompress(y[12:])
 row={'section':name,'baseline_bytes':len(x),'candidate_bytes':len(y),'equal':x==y}
 y=y.replace(ids[1],ids[0]);row['equal_after_build_id']=x==y
 if len(x)==len(y):row['different_bytes_after_build_id']=sum(a!=b for a,b in zip(x,y))
 row['baseline_sha256']=hashlib.sha256(x).hexdigest();row['candidate_sha256']=hashlib.sha256(y).hexdigest();rows.append(row)
(root/'binary-section-comparison.json').write_text(json.dumps(rows,indent=2));print(json.dumps(rows,indent=2))
