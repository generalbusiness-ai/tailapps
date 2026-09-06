import json,pathlib,subprocess,hashlib,os,tempfile,sys
w=pathlib.Path('/Users/hughpyle/play/tailapp-worktrees/resident-5d84-pending');ref=pathlib.Path('/var/folders/2x/wylr59t17ds36l1l7ng25y7w0000gn/T/tail-provenance-3919-bh2yivpi');root=pathlib.Path(tempfile.mkdtemp(prefix='tail3925-build-'));pathlib.Path('/tmp/tail3925-build-session.json').write_text(json.dumps({'root':str(root),'source':str(w),'reference':str(ref)},indent=2))
env=os.environ.copy();overrides={'GOOS':'darwin','GOARCH':'arm64','GOARM64':'v8.0','CGO_ENABLED':'1','GOTOOLCHAIN':'local','GOWORK':'off','GOFLAGS':'','GOPROXY':'off','GOSUMDB':'off'};env.update(overrides)
def run(a,cwd=w):return subprocess.check_output(a,cwd=cwd,env=env,text=True).strip()
raw=run(['go','list','-deps','-json','./cmd/tailapp']);dec=json.JSONDecoder();pkgs=[];i=0
while i<len(raw):
 while i<len(raw) and raw[i].isspace():i+=1
 if i==len(raw):break
 p,n=dec.raw_decode(raw,i);pkgs.append(p);i=n
manifest={}
for p in pkgs:
 for kind in ['GoFiles','CgoFiles','CFiles','CXXFiles','MFiles','HFiles','FFiles','SFiles','SwigFiles','SwigCXXFiles','SysoFiles','EmbedFiles']:
  for f in p.get(kind,[]):
   q=pathlib.Path(p['Dir'])/f;b=q.read_bytes();manifest[p['ImportPath']+'/'+f]={'sha256':hashlib.sha256(b).hexdigest(),'bytes':len(b)}
old=json.load(open(ref/'reference-input-manifest.json'));prior={p['package']+'/'+f:{'sha256':v['sha256'],'bytes':v['bytes']} for p in old['packages'] for f,v in p['files'].items()};changed={k:{'reference':prior.get(k),'candidate':manifest.get(k)} for k in sorted(set(prior)|set(manifest)) if prior.get(k)!=manifest.get(k)}
assert list(changed)==['github.com/generalbusiness-ai/tailapps/internal/inbox/inbox.go'],changed
info={'head':run(['git','rev-parse','HEAD']),'tree':run(['git','rev-parse','HEAD^{tree}']),'status':run(['git','status','--porcelain']),'packages':len(pkgs),'inputs':manifest,'changed_from_accepted_reference':changed,'environment':overrides,'modules':run(['go','list','-m','all'])};assert info['status']=='',info['status']
for f in ['go.mod','go.sum']:info[f]={'sha256':hashlib.sha256((w/f).read_bytes()).hexdigest(),'equal_baseline':run(['git','diff','5d84ac71','HEAD','--',f])==''}
oldtools=json.load(open(ref/'reference-toolchain.json'));info['toolchain']={}
for p,h in oldtools.items():
 if p.startswith('/'):
  current=hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest();assert current==h;info['toolchain'][p]=current
(root/'input-manifest.json').write_text(json.dumps(info,indent=2));print('manifest',root,'inputs',len(manifest),'changed',list(changed),flush=True)
flags=['-trimpath','-ldflags','-X github.com/generalbusiness-ai/tailapps/internal/buildinfo.stampedSourceURL=https://github.com/generalbusiness-ai/tailapps']
commands=[]
for n in [1,2]:
 output=root/f'tailapp-{n}';argv=['go','build',*flags,'-o',str(output),'./cmd/tailapp'];commands.append({'argv':argv,'cwd':str(w),'environment':overrides})
 with (root/f'build-{n}.log').open('w') as log:subprocess.run(argv,cwd=w,env=env,stdout=log,stderr=subprocess.STDOUT,check=True)
 print('built',n,hashlib.sha256(output.read_bytes()).hexdigest(),flush=True)
assert (root/'tailapp-1').read_bytes()==(root/'tailapp-2').read_bytes()
(root/'build-commands.json').write_text(json.dumps(commands,indent=2));(root/'binary-result.json').write_text(json.dumps({'sha256':hashlib.sha256((root/'tailapp-1').read_bytes()).hexdigest(),'bytes':(root/'tailapp-1').stat().st_size,'byte_identical_repeat':True,'build_id':run(['go','tool','buildid',str(root/'tailapp-1')]),'build_info':run(['go','version','-m',str(root/'tailapp-1')])},indent=2))
print('complete',root,flush=True)
