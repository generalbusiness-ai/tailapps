import subprocess,tempfile,pathlib,os,json,tarfile,io
source='/Users/hughpyle/play/tailapp-worktrees/root-module-v020-alignment'
archive=subprocess.check_output(['git','archive','3606dd8a7a65e2dd4aa5531202b8147a8d562229'],cwd=source)
base=pathlib.Path(tempfile.mkdtemp(prefix='tail-root-v020-omissions-'))
results=[]
for case,expected in [('old-pin','root module must select the exact public'),('missing-workspace-module','paired development must use this checkout'),('local-replacement','root module must select the exact public')]:
 p=base/case;p.mkdir();tarfile.open(fileobj=io.BytesIO(archive)).extractall(p,filter='data')
 if case=='old-pin':
  f=p/'go.mod';s=f.read_text();assert 'jsonataddl v0.2.0' in s;f.write_text(s.replace('jsonataddl v0.2.0','jsonataddl v0.1.2'))
 elif case=='missing-workspace-module':
  f=p/'go.work';s=f.read_text();assert '\t./jsonataddl\n' in s;f.write_text(s.replace('\t./jsonataddl\n',''))
 else:
  f=p/'go.mod';f.write_text(f.read_text()+'\nreplace github.com/generalbusiness-ai/tailapps/jsonataddl => ./jsonataddl\n')
 env=os.environ.copy();env.update(GOTOOLCHAIN='go1.26.7',GOENV='off',GOFLAGS='',GOPROXY='https://proxy.golang.org',GOSUMDB='sum.golang.org')
 r=subprocess.run(['sh','scripts/check-root-module.sh'],cwd=p,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (base/(case+'.log')).write_text(r.stdout)
 assert r.returncode!=0 and expected in r.stdout,(case,r.returncode,r.stdout)
 results.append({'omission':case,'exit':r.returncode,'expected_refusal':expected,'log':str(base/(case+'.log'))})
 print(case,'expected refusal PASS',flush=True)
summary={'candidate':'3606dd8a7a65e2dd4aa5531202b8147a8d562229','scratch':str(base),'results':results}
pathlib.Path('/tmp/tail-root-v020-omissions.json').write_text(json.dumps(summary,indent=2))
print(json.dumps(summary,indent=2))
