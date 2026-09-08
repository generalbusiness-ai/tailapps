import hashlib, io, json, os, pathlib, subprocess, tarfile, tempfile
repo=pathlib.Path('/Users/hughpyle/play/tailapp');source=(repo/'scripts/install.sh').read_bytes();head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip();rows=[]
with tempfile.TemporaryDirectory(prefix='planner-installer-remedy-') as td:
 root=pathlib.Path(td);fake=root/'fake';fake.mkdir()
 def exe(path,body):path.write_text(body);path.chmod(0o700)
 exe(fake/'uname','#!/bin/sh\ncase "$1" in -s) echo Darwin;; -m) echo arm64;; *) exit 2;; esac\n')
 exe(fake/'launchctl','#!/bin/sh\nprintf "%s\\n" "$*" >>"$PROBE_SERVICE_CALLS"\nexit 1\n')
 exe(fake/'cosign','#!/bin/sh\nexit 0\n')
 release=root/'release/download/v9.9.9';release.mkdir(parents=True)
 stub=b'''#!/bin/sh
printf '%s\\n' "$*" >>"$PROBE_BINARY_CALLS"
case "$1" in version) echo '{"version":"9.9.9"}';; init) exit 0;; *) exit 91;; esac
'''
 archive=release/'tailapps_9.9.9_darwin_arm64.tar.gz'
 with tarfile.open(archive,'w:gz') as tar:
  info=tarfile.TarInfo('tailapp');info.size=len(stub);info.mode=0o755;tar.addfile(info,io.BytesIO(stub))
 (release/'checksums.txt').write_text(hashlib.sha256(archive.read_bytes()).hexdigest()+'  '+archive.name+'\n')
 for name in ['checksums.txt.sig','checksums.txt.bundle']:(release/name).write_text('mock signature boundary\n')
 script=root/'install.sh';script.write_bytes(source.replace(b'@TAILAPPS_VERSION@',b'9.9.9'))
 for name,flags in [('service_unavailable',['--bundles','none']),('explicit_no_service',['--bundles','none','--no-service'])]:
  trial=root/name;trial.mkdir();home=trial/'home';home.mkdir();install=trial/'local';state=trial/'data';calls=trial/'calls';service=trial/'service-calls'
  env=dict(os.environ,HOME=str(home),TAILAPPS_INSTALL_ROOT=str(install),TAILAPP_HOME=str(state),TAILAPPS_RELEASE_BASE_URL=(root/'release').as_uri(),PROBE_BINARY_CALLS=str(calls),PROBE_SERVICE_CALLS=str(service),PATH=str(fake)+':'+os.environ['PATH'])
  p=subprocess.run(['sh',str(script),*flags],env=env,text=True,capture_output=True,timeout=10);link=install/'bin/tailapp'
  row={'case':name,'exit_code':p.returncode,'stdout':p.stdout,'stderr':p.stderr,'binary_retained':link.is_symlink() and link.resolve().exists(),'binary_calls':calls.read_text().splitlines(),'mock_service_calls':service.read_text().splitlines() if service.exists() else [],'foreground_command_present':' serve --otlp-http ' in p.stdout+p.stderr};rows.append(row)
result={'head':head,'source_sha256':hashlib.sha256(source).hexdigest(),'method':'Complete pinned installer, local file:// archive with actual checksum verification, stub binary and cosign, fake launchctl that always refuses, actual plist validation. All install/service paths temporary; no real service or app writes. No cryptographic verification claimed.','results':rows}
pathlib.Path('/tmp/planner-tail-installer-remedy-probe-20260908.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
