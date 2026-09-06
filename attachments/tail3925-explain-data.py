import runpy,contextlib,io,json,subprocess,struct,pathlib
with contextlib.redirect_stdout(io.StringIO()):x=runpy.run_path('/tmp/tail3925-compare-sections.py')
assert all(row.get('equal_after_build_id', row.get('zero_fill_equal',False)) for row in x['rows'] if row['section'] not in {'__DATA,__go_buildinfo','__DATA,__go_module','__TEXT,__rodata','__TEXT,__gopclntab','__DWARF,__zdebug_info','__DWARF,__zdebug_line'}), 'unexpected code or data difference'
r=x['root'];files=x['files'];data=x['data'];ss=x['ss'];ids=x['ids']
def section(n,i):v=ss[i][n];return data[i][v[4]:v[4]+v[3]]
def vcs(p):
 out={}
 for line in subprocess.check_output(['go','version','-m',str(p)],text=True).splitlines():
  if line.startswith('\tmod\t'):out['module.version']=line.split('\t')[3]
  if '\tbuild\tvcs.' in line:
   k,v=line.strip().split('\t',1)[1].split('=',1);out[k]=v
 return out
v=[vcs(p) for p in files];replace=[(ids[1],ids[0])]+[(v[1][k].encode(),v[0][k].encode()) for k in ['vcs.revision','vcs.time','module.version']]
rows=[]
for name in ['__DATA,__go_buildinfo','__TEXT,__rodata']:
 a,b=section(name,0),section(name,1)
 for f,t in replace:b=b.replace(f,t)
 positions=[i for i,(f,t) in enumerate(zip(a,b)) if f!=t];row={'section':name,'differences_after_build_identity':len(positions)}
 if name.endswith('__rodata'):
  assert len(positions)==1;at=positions[0];assert a[at:at+10]==b'e.position' and b[at:at+10]==b'o.position';row['intentional_change']='ORDER BY e.position -> ORDER BY o.position';row['offset']=at
 else:
  if a!=b: print("buildinfo remaining",[(i,a[max(0,i-15):i+30],b[max(0,i-15):i+30]) for i in positions[:3]])
  assert a==b
 rows.append(row)
a,b=section('__DATA,__go_module',0),section('__DATA,__go_module',1);words=[]
for i in range(0,len(a),8):
 u,vv=struct.unpack_from('<Q',a,i)[0],struct.unpack_from('<Q',b,i)[0]
 if u!=vv:words.append({'offset':i,'baseline':u,'candidate':vv,'delta':vv-u})
expected={88:'pctab length',96:'pctab capacity',104:'pclntable pointer',128:'ftab pointer',152:'findfunctab pointer',344:'gofunc pointer',352:'epclntab pointer'}
assert {w['offset'] for w in words}==set(expected) and all(w['delta']==-8 for w in words)
for word in words:word['field']=expected[word['offset']]
result={'build_identity':v,'sections':rows,'module_descriptor_changed_words':words,'all_machine_instructions_equal_after_build_id':True};(r/'binary-data-comparison.json').write_text(json.dumps(result,indent=2));print(json.dumps(result,indent=2))
