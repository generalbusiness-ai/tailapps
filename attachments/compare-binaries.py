import argparse, hashlib, json, pathlib, struct, subprocess, zlib

p=argparse.ArgumentParser()
p.add_argument('original');p.add_argument('reference');p.add_argument('--map',nargs=2,action='append',default=[]);p.add_argument('--output',required=True)
a=p.parse_args();original=pathlib.Path(a.original).read_bytes();reference=pathlib.Path(a.reference).read_bytes()
ids=[subprocess.check_output(['go','tool','buildid',f],text=True).strip() for f in [a.original,a.reference]]
replacements=[(x.encode(),y.encode()) for x,y in a.map]+[(ids[1].encode(),ids[0].encode())]
assert all(len(x)==len(y) for x,y in replacements)
def parse(data):
    assert struct.unpack_from('<I',data)[0]==0xfeedfacf
    pos=32;sections={};commands=[]
    for _ in range(struct.unpack_from('<I',data,16)[0]):
        cmd,n=struct.unpack_from('<II',data,pos);item={'cmd':cmd,'offset':pos,'size':n}
        if cmd==0x19:
            segment=struct.unpack_from('<16sQQQQIIII',data,pos+8);item['segment']=segment
            for j in range(segment[7]):
                off=pos+72+j*80;v=struct.unpack_from('<16s16sQQIIIIIIII',data,off)
                name=v[1].split(b'\0')[0].decode()+','+v[0].split(b'\0')[0].decode();sections[name]=(off,v)
        commands.append(item);pos+=n
    return sections,commands
sa,ca=parse(original);sb,cb=parse(reference);assert sa.keys()==sb.keys();assert len(original)==len(reference)
mask=bytearray(len(original));rows=[]
def cover(start,size):mask[start:start+size]=b'\1'*size
def norm(data):
    for x,y in replacements:data=data.replace(x,y)
    return data
for name in sorted(sa):
    off,x=sa[name];_,y=sb[name];row={'section':name,'original_size':x[3],'reference_size':y[3]}
    assert x[:2]==y[:2] and x[5:]==y[5:]
    if x[8]&255 in [1,12,18]:
        assert x==y;row['zero_fill_equal']=True;rows.append(row);continue
    u=original[x[4]:x[4]+x[3]];v=reference[y[4]:y[4]+y[3]];row['raw_equal']=u==v
    if u.startswith(b'ZLIB') or v.startswith(b'ZLIB'):
        assert u.startswith(b'ZLIB') and v.startswith(b'ZLIB');u=zlib.decompress(u[12:]);v=zlib.decompress(v[12:]);row['decompressed']=True
    assert u==norm(v),name
    row['normalized_equal']=True;row['normalized_sha256']=hashlib.sha256(u).hexdigest();row['replacements']=[{'from':x.decode(),'to':y.decode(),'count':v.count(x)} for x,y in replacements if x in v]
    if name.startswith('__DWARF,'):
        # Only size and the corresponding debug address/file offset may move.
        assert x[2]-x[4]==y[2]-y[4]
    else:
        assert x==y
        if not row['raw_equal']:cover(x[4],x[3])
    rows.append(row)
checks=[]
for x,y in zip(ca,cb):
    assert (x['cmd'],x['offset'],x['size'])==(y['cmd'],y['offset'],y['size']);off=x['offset'];n=x['size'];cmd=x['cmd']
    if cmd==0x19 and x['segment'][0].startswith(b'__DWARF'):
        sx=x['segment'];sy=y['segment'];assert sx[:4]==sy[:4] and sx[5:]==sy[5:];assert sx[2]==sx[5]==sx[6]==0
        assert sy[4]-sx[4]==sum(sb[k][1][3]-sa[k][1][3] for k in sa if k.startswith('__DWARF,'))
        for data,sections,seg in [(original,sa,sx),(reference,sb,sy)]:
            cursor=seg[3]
            for _,v in sorted((z for k,z in sections.items() if k.startswith('__DWARF,')),key=lambda z:z[1][4]):
                assert not any(data[cursor:v[4]]);cursor=v[4]+v[3]
            assert not any(data[cursor:seg[3]+max(sx[4],sy[4])])
        cover(off,n);cover(sx[3],max(sx[4],sy[4]));checks.append({'dwarf_segment_unmapped':True,'compressed_size_delta':sy[4]-sx[4]})
    elif cmd==0x1b:
        assert original[off:off+8]==reference[off:off+8]
        for data,bid in [(original,ids[0]),(reference,ids[1])]:
            parts=bid.split('/');initial='/'.join(parts[:3]+[parts[0]]);h=bytearray(hashlib.sha256(initial.encode()).digest()[:16]);h[0]^=255;h[6]=(h[6]&15)|48;h[8]=(h[8]&63)|128;assert data[off+8:off+24]==h
        cover(off+8,16);checks.append({'uuid_matches_go_initial_build_id':True})
    elif cmd==0x1d:
        assert original[off:off+n]==reference[off:off+n];sigoff,sigsize=struct.unpack_from('<II',original,off+8)
        metadata=[]
        for data in [original,reference]:
            magic,length,count=struct.unpack_from('>III',data,sigoff);assert magic==0xfade0cc0 and count==1 and length==sigsize
            slot,cdoff=struct.unpack_from('>II',data,sigoff+12);assert slot==0
            cd=sigoff+cdoff;v=struct.unpack_from('>9I4B',data,cd);assert v[0]==0xfade0c02 and v[6]==0 and v[9:]==(32,2,0,12)
            hstart=cd+v[4];hend=hstart+v[7]*32;assert hend==sigoff+sigsize and v[8]==sigoff
            for i in range(v[7]):assert data[hstart+i*32:hstart+(i+1)*32]==hashlib.sha256(data[i*4096:min((i+1)*4096,v[8])]).digest()
            metadata.append(data[sigoff:hstart]);cover(hstart,hend-hstart)
        assert metadata[0]==metadata[1];checks.append({'signature_metadata_equal':True,'all_sha256_page_hashes_verified':True,'pages_per_binary':v[7]})
    else:assert original[off:off+n]==reference[off:off+n],hex(cmd)
unexplained=[i for i,(x,y) in enumerate(zip(original,reference)) if x!=y and not mask[i]]
assert not unexplained,unexplained[:20]
out={'original':a.original,'reference':a.reference,'original_sha256':hashlib.sha256(original).hexdigest(),'reference_sha256':hashlib.sha256(reference).hexdigest(),'file_size':len(original),'build_ids':ids,'sections':rows,'checks':checks,'unexplained_different_bytes':len(unexplained),'conclusion':'All bytes agree or are exhaustively accounted for by verified path/build-ID normalization, equivalent decoded debug sections and their layout, derived UUIDs, and valid signature page hashes. This is not a raw byte-identical rebuild or an original dirty-source manifest.'}
pathlib.Path(a.output).write_text(json.dumps(out,indent=2));print(json.dumps({'sections':len(rows),'unexplained_different_bytes':len(unexplained),'checks':checks}))
