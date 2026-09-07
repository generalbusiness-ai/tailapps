from pathlib import Path
import json,difflib,sys,subprocess,os
root=Path(sys.argv[1]).resolve();core=Path(sys.argv[2]).resolve();outdir=Path(sys.argv[3]).resolve();outdir.mkdir(parents=True,exist_ok=True)
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()=='311d9afc88eddf666dc1499ffcdaf35a3b00cb32'
nums=['0/0','1/0','-1/0','1e300','-1e300','9223372036854775808','9223372036854774784','9223372036854777856','-9223372036854775808','-9223372036854774784','-9223372036854777856','0','-0','1.9','-1.9','0.5','-0.5','missing']
cases=[]
for n in nums:
 for src in [f'$round(1.25,{n})',f'$substring("abc",{n})',f'$substring("abc",0,{n})',f'$substring("abc",1,{n})',f'$substring("abc",-1,{n})',f'[1,2,3][{n}]',f'($sum:=$round(? ,{n});$sum(1.25))',f'($sum:=$substring(?,1,{n});$sum("abc"))']:
  cases.append(src)
cases += ['[$round(1.25,0/0),$substring("abc",0,1e300)]','[$round(1.25,0/0),$substring("abc",1,1e300),$round(2.25,0)]','$round($number("invalid"),0/0)','$substring($string(7),$floor(1.9),$ceil(1.9))','$round(1e300,1e300)','$round(1e300,-1e300)','$round(1e-300,1e300)','$round(1e-300,-1e300)','$round(missing,0/0)','$round(1.25)','$round(1.25,missing)','$substring("",1,1e300)','$substring(missing,1,1e300)','$substring("abc",0,missing)','$substring("abc",1.9,1.9)','$substring("abc",-1.9,1.9)','[1,2,3][[0/0]]','[1,2,3][[1/0]]','[1,2,3][[-1e300,1.9,1e300]]','($sum:=$round;$sum(1.25,0/0))','($sum:=$substring;$sum("abc",0,1e300))','1.25 ~> $round(0/0)','"abc" ~> $substring(0,1e300)','$number("0x7fffffffffffffff")','$number("0x8000000000000000")','$number("0b111")','$string(1e300)','$string(1e-300)','$floor(1.9)','$ceil(-1.9)']
Path(str(outdir/'cases.json')).write_text(json.dumps(cases,indent=2))
literals=',\n'.join('`'+s+'`' for s in cases)+','
probe='''package main
import("encoding/json";"fmt";j "github.com/jsonata-go/jsonata/v206")
func main(){for _,source:=range []string{CASES
}{e,err:=j.Compile(source,false);if err!=nil{panic(err)};out,err:=e.Evaluate([]byte(`null`),nil);failure:="";if err!=nil{failure=err.Error()};b,_:=json.Marshal(map[string]interface{}{"source":source,"output":string(out),"error":failure});fmt.Println(string(b))}}
'''.replace('CASES',literals)
Path(str(outdir/'native-probe.go')).write_text(probe)
original=(root/'v206/meter_evaluate_test.go').read_text().replace('"bytes"','"bytes"\n "encoding/json"')
extra='''
var numericProofCalls []string
func TestNumericDecisionEvidence(t *testing.T) {
 for _,source:=range []string{CASES
 } {
  program,err:=compileResourceProgram(source);if err!=nil{t.Fatal(source,err)}
  limits:=ResourceLimits{1<<44,1<<40,128};budget:=NewResourceBudget(limits);scope:=budget.Scope(limits)
  numericProofCalls=[]string{};out,err:=evaluateResourceProgram(&scope,program,[]byte(`null`));failure:="";if err!=nil{failure=err.Error()}
  calls:=append([]string{},numericProofCalls...)
  bounds:=map[string]string{}
  if err==nil { for _,which:=range []string{"exact","work-one-short","allocation-one-short"} {
   bound:=budget.Used();bound.Depth=128;want:=error(nil)
   if which=="work-one-short"{bound.Work--;want=ResourceWork};if which=="allocation-one-short"{bound.Allocation--;want=ResourceAllocation}
   b:=NewResourceBudget(bound);s:=b.Scope(bound);numericProofCalls=[]string{};got,e:=evaluateResourceProgram(&s,program,[]byte(`null`))
   if e!=want || (want==nil && !bytes.Equal(got,out)) || (want!=nil && got!=nil) || s.depth!=0 {t.Fatalf("bound %s %s: %s/%v want %v",source,which,got,e,want)}
   bounds[which]="success";if e!=nil{bounds[which]=e.Error()}
  }}
  b,_:=json.Marshal(map[string]interface{}{"source":source,"output":string(out),"error":failure,"calls":calls,"used":budget.Used(),"bounds":bounds});t.Logf("RESULT %s",b)
 }
}
'''.replace('CASES',literals)
Path(str(outdir/'private-test.go')).write_text(original+extra)
invoke=(root/'v206/meter_invoke.go').read_text(); needle='\tvar body func([]interface{}) (interface{}, error)';assert invoke.count(needle)==1
invoke=invoke.replace(needle,'\tnumericProofCalls=append(numericProofCalls,name)\n'+needle)
Path(str(outdir/'invoke-trace.go')).write_text(invoke)
common={str(root/'v206/meter_evaluate_test.go'):str(outdir/'private-test.go'),str(root/'v206/meter_invoke.go'):str(outdir/'invoke-trace.go')}
Path(str(outdir/'private-native-overlay.json')).write_text(json.dumps({'Replace':common}))
base=(root/'v206/functions.go').read_text();portable=base
for a,b in [('start = int(s)','start = numericDecisionInt(scope,s)'),('val := int(l)','val := numericDecisionInt(scope,l)'),('val := int(p)','val := numericDecisionInt(scope,p)')]:
 # start occurs in excluded helper too: only replace first, substring is first.
 assert a in portable;portable=portable.replace(a,b,1)
portable+='''
// Evidence-only proposed semantic conversion; not a delivered implementation.
func numericDecisionInt(scope *ResourceScope,n float64) int {
 if scope!=nil {requireResources(scope,8,resourceScalarUnits)}
 if math.IsNaN(n){return 0}
 if n>=9223372036854775808.0{return 9223372036854775807}
 if n<=-9223372036854775808.0{return -9223372036854775808}
 return int(n)
}
'''
# Retain failure at same slice point, before native bounds panic. Scope-safe fixed failure.
portable=portable.replace('return joinResourceCharacters(scope, strArray[start:end]), nil','if start < 0 || end < start || end > len(strArray) { panic(scope.fail(ResourceContract)) }; return joinResourceCharacters(scope, strArray[start:end]), nil',1)
portable=portable.replace('return joinResourceCharacters(scope, strArray[start:]), nil','if start < 0 || start > len(strArray) { panic(scope.fail(ResourceContract)) }; return joinResourceCharacters(scope, strArray[start:]), nil',1)
Path(str(outdir/'portable-functions.go')).write_text(portable)
filterbase=(root/'v206/meter_filter.go').read_text()
a='''case float64:
			requireResources(scope, 1, resourceFrameUnits+resourceScalarUnits)
			position, valid := resourceFilterIndex'''
b='''case float64:
            if math.IsNaN(value) {
                requireResources(scope,1,resourceFrameUnits+resourceScalarUnits)
                selected=resourceBoolize(environment,value)
                break
            }
			requireResources(scope, 1, resourceFrameUnits+resourceScalarUnits)
			position, valid := resourceFilterIndex'''
assert filterbase.count(a)==1
filternew=filterbase.replace(a,b)
Path(str(outdir/'portable-filter.go')).write_text(filternew)
new=dict(common);new[str(root/'v206/functions.go')]=str(outdir/'portable-functions.go');new[str(root/'v206/meter_filter.go')]=str(outdir/'portable-filter.go')
Path(str(outdir/'private-portable-overlay.json')).write_text(json.dumps({'Replace':new}))
diff=''.join(difflib.unified_diff(base.splitlines(True),portable.splitlines(True),fromfile='v206/functions.go',tofile='evidence-overlay/functions.go'))+''.join(difflib.unified_diff(filterbase.splitlines(True),filternew.splitlines(True),fromfile='v206/meter_filter.go',tofile='evidence-overlay/meter_filter.go'))
Path(str(outdir/'portable-overlay.diff')).write_text(diff)
print('cases',len(cases),'overlays only; no tracked source changed')

# The exact omission assertions are separately signed beside this script.
omission_test=Path(__file__).with_name('omissions-test.go').read_text()
(outdir/'omissions-test.go').write_text(omission_test)
control=dict(new);control[str(root/'v206/meter_evaluate_test.go')]=str(outdir/'omissions-test.go')
(outdir/'omissions-control.json').write_text(json.dumps({'Replace':control}))
variants={
 'native-cast':portable.replace('if math.IsNaN(n){return 0}\n if n>=9223372036854775808.0{return 9223372036854775807}\n if n<=-9223372036854775808.0{return -9223372036854775808}\n',''),
 'intel-cast-control':portable.replace('if math.IsNaN(n){return 0}','if math.IsNaN(n){return -9223372036854775808}').replace('if n>=9223372036854775808.0{return 9223372036854775807}','if n>=9223372036854775808.0{return -9223372036854775808}'),
 'skip-charge':portable.replace('if scope!=nil {requireResources(scope,8,resourceScalarUnits)}',''),
 'slice-guard':portable.replace('if start < 0 || end < start || end > len(strArray) { panic(scope.fail(ResourceContract)) }; ',''),
 'ordinary-error':portable.replace('panic(scope.fail(ResourceContract)) }; return joinResourceCharacters','failResourceExpression(scope) }; return joinResourceCharacters'),
}
for name,value in variants.items():
 p=outdir/('omit-'+name+'.go');p.write_text(value);ov=dict(control);ov[str(root/'v206/functions.go')]=str(p);(outdir/('omit-'+name+'.json')).write_text(json.dumps({'Replace':ov}))
ov=dict(control);ov.pop(str(root/'v206/meter_filter.go'));(outdir/'omit-predicate-guard.json').write_text(json.dumps({'Replace':ov}))
# Preserve host/target distinction. Run only on an ARM64 Go1.26.7 host for the
# recorded native-cast survival; no result here is AMD64 evaluator execution.
env=dict(os.environ,GOWORK='off',CGO_ENABLED='0')
metadata=subprocess.check_output(['go','env','GOVERSION','GOOS','GOARCH'],cwd=root,env=env,text=True)
(outdir/'environment.txt').write_text(metadata)
commands=[('native',core,['go','run',str(outdir/'native-probe.go')],0)]
for name in ['native','portable']:
 commands.append(('private-'+name,root,['go','test','-overlay',str(outdir/('private-'+name+'-overlay.json')),'./v206','-run','^TestNumericDecisionEvidence$','-count=1','-v'],0))
for name,expected in [('omissions-control',0),('omit-native-cast',0),('omit-intel-cast-control',1),('omit-skip-charge',1),('omit-predicate-guard',1)]:
 commands.append((name,root,['go','test','-overlay',str(outdir/(name+'.json')),'./v206','-run','^TestNumericDecisionOmissions$','-count=1','-v'],expected))
for name,expected in [('omissions-control',0),('omit-slice-guard',1),('omit-ordinary-error',1)]:
 commands.append(('protected-'+name,root,['go','test','-overlay',str(outdir/(name+'.json')),'./v206','-run','^TestNumericDecisionProtectedSlice$','-count=1','-v'],expected))
results=[]
for name,cwd,cmd,expected in commands:
 with (outdir/(name+'.log')).open('w') as log:r=subprocess.run(cmd,cwd=cwd,env=env,stdout=log,stderr=subprocess.STDOUT)
 results.append({'name':name,'command':cmd,'cwd':str(cwd),'expected_status_on_recorded_ARM_host':expected,'actual_status':r.returncode})
 print(name,r.returncode,flush=True)
(outdir/'replay-results.json').write_text(json.dumps(results,indent=2))
assert all(x['actual_status']==x['expected_status_on_recorded_ARM_host'] for x in results)
