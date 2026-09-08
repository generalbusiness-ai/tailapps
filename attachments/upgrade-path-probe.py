import json, os, pathlib, subprocess, tempfile, hashlib, plistlib, shlex
repo=pathlib.Path('/Users/hughpyle/play/tailapp')
head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()
source=(repo/'scripts/upgrade.sh').read_bytes()
results=[]
with tempfile.TemporaryDirectory(prefix='planner-upgrade-path-') as td:
 root=pathlib.Path(td); fake=root/'fakebin';fake.mkdir()
 def executable(path,text):
  path.write_text(text);path.chmod(0o700)
 executable(fake/'uname','#!/bin/sh\nprintf "%s\\n" Darwin\n')
 executable(fake/'launchctl','#!/bin/sh\n[ "$1" = print ]\n')
 script=root/'upgrade.sh';script.write_bytes(source.replace(b'@TAILAPPS_VERSION@',b'9.9.9'))
 for label,suffix in [('plain','data'),('space','data space'),('quote','data"quote'),('backslash',r'data\slash')]:
  trial=root/label;trial.mkdir(); home=trial/'user';home.mkdir(); install=trial/'local';lib=install/'lib/tailapp';lib.mkdir(parents=True);bindir=install/'bin';bindir.mkdir()
  state=trial/suffix;state.mkdir();target=lib/'tailapp-9.9.9'
  executable(target,'#!/bin/sh\n[ "$1" = health ] || exit 2\nprintf "%s\\n" \'{"control_plane":"healthy","ingestion_ready":false}\'\n')
  link=bindir/'tailapp';link.symlink_to(target)
  unit=home/'Library/LaunchAgents/ai.generalbusiness.tailapp.plist';unit.parent.mkdir(parents=True)
  unit.write_bytes(plistlib.dumps({'ProgramArguments':[str(link),'serve','--otlp-http','127.0.0.1:4318'],'EnvironmentVariables':{'TAILAPP_HOME':str(state)}}))
  env=dict(os.environ,HOME=str(home),XDG_CONFIG_HOME=str(home/'.config'),TAILAPPS_INSTALL_ROOT=str(install),TAILAPP_HOME=str(state),PATH=str(fake)+':'+os.environ['PATH'])
  p=subprocess.run(['sh',str(script)],env=env,capture_output=True,text=True,timeout=5)
  try:
   decoded=json.loads(p.stdout);error=None
  except json.JSONDecodeError as e:
   decoded=None;error=str(e)
  results.append(dict(case=label,exit_code=p.returncode,stdout=p.stdout,stderr=p.stderr,json_valid=error is None,json_error=error,decoded=decoded,link_unchanged=os.readlink(link)==str(target),next_command_tokens=shlex.split(decoded['next'].split(';',1)[0]) if decoded else None,expected_command_tokens=['TAILAPP_HOME='+str(state),str(link),'apps','status']))
report=dict(head=head,path='scripts/upgrade.sh',sha256=hashlib.sha256(source).hexdigest(),method='Run full pinned copy of actual upgrade script at same-version path with fake launchctl print boundary, actual macOS plutil parsing a valid generated LaunchAgent, and fake health binary. No downloads, real services, upgrades or app writes. Generated plist preserves exact binary and home. No actual launchd configuration or service is touched.',results=results)
path=pathlib.Path('/tmp/planner-tail-upgrade-path-probe-20260908.json');path.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
